package transcode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"videoview/internal/config"
	"videoview/internal/model"
	"videoview/internal/repository"
	"videoview/internal/upload"
)

type ProbeInfo struct {
	Width    int
	Height   int
	Duration time.Duration
}

type Worker struct {
	cfg    config.TranscodeConfig
	store  *upload.Store
	videos *repository.VideoRepo
	log    *zap.Logger
	notify chan uint64
}

func NewWorker(cfg config.TranscodeConfig, store *upload.Store, videos *repository.VideoRepo, log *zap.Logger) *Worker {
	return &Worker{
		cfg:    cfg,
		store:  store,
		videos: videos,
		log:    log.Named("transcode"),
		notify: make(chan uint64, 64),
	}
}

func (w *Worker) Notify(id uint) {
	select {
	case w.notify <- uint64(id):
	default:
		w.log.Warn("notify queue full", zap.Uint("video_id", id))
	}
}

func (w *Worker) Start(ctx context.Context) {
	jobs := make(chan uint64, w.cfg.Concurrency*2)
	for i := 0; i < w.cfg.Concurrency; i++ {
		go w.loop(ctx, jobs)
	}
	go w.feed(ctx, jobs)
}

func (w *Worker) feed(ctx context.Context, jobs chan<- uint64) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-w.notify:
			select {
			case jobs <- id:
			case <-ctx.Done():
				return
			}
		case <-ticker.C:
			ids, err := w.videos.ListUploadedIDs(ctx)
			if err != nil {
				w.log.Error("list uploaded", zap.Error(err))
				continue
			}
			for _, id := range ids {
				select {
				case jobs <- uint64(id):
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (w *Worker) loop(ctx context.Context, jobs <-chan uint64) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-jobs:
			if err := w.Process(ctx, uint(id)); err != nil {
				w.log.Error("process video", zap.Uint64("id", id), zap.Error(err))
			}
		}
	}
}

func (w *Worker) Process(ctx context.Context, id uint) error {
	ok, err := w.videos.ClaimUploaded(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	v, err := w.videos.FindByID(ctx, id)
	if err != nil {
		return err
	}
	info, err := w.Probe(ctx, v.SourcePath)
	if err != nil {
		_ = w.videos.MarkFailed(ctx, id, "ffprobe failed")
		return fmt.Errorf("probe: %w", err)
	}
	v.Width = info.Width
	v.Height = info.Height
	v.DurationMS = info.Duration.Milliseconds()
	if err := w.videos.Save(ctx, v); err != nil {
		return err
	}

	targets := PlanQualities()
	renditions := make([]model.VideoRendition, 0, len(targets))
	for _, q := range targets {
		renditions = append(renditions, model.VideoRendition{
			VideoID: id,
			Quality: strconv.Itoa(q),
			Width:   evenScaleWidth(info.Width, info.Height, q),
			Height:  q,
			Status:  model.RenditionPending,
		})
	}
	if err := w.videos.ReplaceRenditions(ctx, id, renditions); err != nil {
		return err
	}
	v, err = w.videos.FindByID(ctx, id)
	if err != nil {
		return err
	}

	encodeCount := 0
	for _, r := range v.Renditions {
		if r.Status == model.RenditionPending {
			encodeCount++
		}
	}
	if encodeCount == 0 {
		_ = w.videos.UpdateProgress(ctx, id, -1, 100, "")
		return w.videos.MarkReady(ctx, id)
	}

	done := 0
	for i := range v.Renditions {
		r := &v.Renditions[i]
		if r.Status != model.RenditionPending {
			continue
		}
		out := w.store.RenditionPath(v.StoredBasename, r.Quality)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			_ = w.videos.MarkFailed(ctx, id, err.Error())
			return err
		}
		h, _ := strconv.Atoi(r.Quality)
		onProg := func(p int) {
			overall := (done*100 + p) / encodeCount
			_ = w.videos.UpdateProgress(ctx, id, -1, overall, "")
		}
		if err := w.Encode(ctx, v.SourcePath, out, h, v.DurationMS, onProg); err != nil {
			r.Status = model.RenditionFailed
			_ = w.videos.SaveRendition(ctx, r)
			_ = w.videos.MarkFailed(ctx, id, "ffmpeg failed: "+r.Quality)
			return err
		}
		r.Path = out
		r.Status = model.RenditionReady
		if err := w.videos.SaveRendition(ctx, r); err != nil {
			return err
		}
		done++
		_ = w.videos.UpdateProgress(ctx, id, -1, done*100/encodeCount, "")
	}
	return w.videos.MarkReady(ctx, id)
}

func PlanQualities() []int {
	return []int{480, 720, 1080}
}

func evenScaleWidth(srcW, srcH, targetH int) int {
	if srcH == 0 {
		return 0
	}
	w := srcW * targetH / srcH
	if w%2 != 0 {
		w--
	}
	return w
}

func (w *Worker) Probe(ctx context.Context, path string) (*ProbeInfo, error) {
	cmd := exec.CommandContext(ctx, w.cfg.FFprobe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Streams []struct {
			Width    int    `json:"width"`
			Height   int    `json:"height"`
			Duration string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	info := &ProbeInfo{}
	if len(parsed.Streams) > 0 {
		info.Width = parsed.Streams[0].Width
		info.Height = parsed.Streams[0].Height
		if d, err := strconv.ParseFloat(parsed.Streams[0].Duration, 64); err == nil {
			info.Duration = time.Duration(d * float64(time.Second))
		}
	}
	if info.Duration == 0 {
		if d, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
			info.Duration = time.Duration(d * float64(time.Second))
		}
	}
	if info.Width == 0 || info.Height == 0 {
		return nil, fmt.Errorf("no video stream")
	}
	return info, nil
}

func (w *Worker) Encode(ctx context.Context, src, dest string, height int, durationMS int64, onProgress func(int)) error {
	args := []string{
		"-y",
		"-i", src,
		"-vf", fmt.Sprintf("scale=-2:%d", height),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-c:a", "aac",
		"-movflags", "+faststart",
		"-progress", "pipe:1",
		"-nostats",
		dest,
	}
	cmd := exec.CommandContext(ctx, w.cfg.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := sc.Text()
		var us int64
		switch {
		case strings.HasPrefix(line, "out_time_us="):
			us, _ = strconv.ParseInt(strings.TrimPrefix(line, "out_time_us="), 10, 64)
		case strings.HasPrefix(line, "out_time_ms="):
			us, _ = strconv.ParseInt(strings.TrimPrefix(line, "out_time_ms="), 10, 64)
		default:
			continue
		}
		if durationMS > 0 && onProgress != nil {
			p := int(us * 100 / (durationMS * 1000))
			if p > 99 {
				p = 99
			}
			if p < 0 {
				p = 0
			}
			onProgress(p)
		}
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	if onProgress != nil {
		onProgress(100)
	}
	return nil
}
