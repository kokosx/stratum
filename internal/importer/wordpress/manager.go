package wordpress

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kokosx/stratum/internal/runtimehub"
)

// WordPressImportManager is a tiny in-process coordinator for one WordPress import at a time.
// It is NOT a generic job framework — it exists only for the rare, long-running WXR import.
type WordPressImportManager struct {
	mu             sync.Mutex
	dataDir        string
	importer       *Importer
	runtime        *runtimehub.Runtime
	jobs           map[string]*Job
	activeImportID string
}

// Job holds state for one import workflow (upload → analyze → import → complete).
type Job struct {
	ID            string
	Phase         string
	StartedAt     time.Time
	FinishedAt    *time.Time
	Report        *Report
	BackupPath    string
	Error         string
	TempPath      string
	DownloadMedia bool
	Author        string
	Done          bool
}

func NewManager(dataDir string, importer *Importer, runtime *runtimehub.Runtime) *WordPressImportManager {
	return &WordPressImportManager{
		dataDir:  dataDir,
		importer: importer,
		runtime:  runtime,
		jobs:     make(map[string]*Job),
	}
}

// Analyze runs a dry-run on the uploaded file synchronously and stores the job for Review.
func (m *WordPressImportManager) Analyze(ctx context.Context, tempPath string, downloadMedia bool, author string) (*Job, error) {
	report, err := m.importer.Analyze(ctx, tempPath, Options{DownloadMedia: downloadMedia, Author: author, DataDir: m.dataDir})
	if err != nil {
		return nil, err
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	job := &Job{
		ID:            id,
		Phase:         "Review",
		StartedAt:     time.Now(),
		Report:        &report,
		TempPath:      tempPath,
		DownloadMedia: downloadMedia,
		Author:        author,
		Done:          false,
	}
	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()
	return job, nil
}

// StartImport begins the real import asynchronously. Only one may run at a time.
func (m *WordPressImportManager) StartImport(ctx context.Context, jobID string) (*Job, error) {
	m.mu.Lock()
	if m.activeImportID != "" {
		if j, ok := m.jobs[m.activeImportID]; ok && !j.Done {
			m.mu.Unlock()
			return nil, fmt.Errorf("another WordPress import is already running")
		}
	}
	job, ok := m.jobs[jobID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("import job not found")
	}
	if job.Done {
		m.mu.Unlock()
		return nil, fmt.Errorf("import already completed")
	}
	// Ensure temp file still exists
	if job.TempPath != "" {
		if _, err := os.Stat(job.TempPath); err != nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("uploaded file missing")
		}
	}
	job.Phase = "Preparing"
	job.StartedAt = time.Now()
	job.Error = ""
	job.Done = false
	m.activeImportID = jobID
	m.mu.Unlock()

	// Async execution
	go func(j *Job) {
		progress := func(phase string) {
			m.mu.Lock()
			j.Phase = phase
			m.mu.Unlock()
		}
		report, backupPath, err := m.importer.Execute(context.Background(), j.TempPath, Options{
			DownloadMedia: j.DownloadMedia,
			Author:        j.Author,
			DataDir:       m.dataDir,
			Progress:      progress,
		})
		m.mu.Lock()
		defer m.mu.Unlock()
		now := time.Now()
		j.FinishedAt = &now
		j.Done = true
		if err != nil {
			j.Error = err.Error()
			j.Phase = "Failed"
		} else {
			j.Report = &report
			j.BackupPath = backupPath
			j.Phase = "Done"
		}
		m.activeImportID = ""
		// Cleanup temp file
		if j.TempPath != "" {
			_ = os.Remove(j.TempPath)
			j.TempPath = ""
		}
		// Runtime refresh
		if err == nil && m.runtime != nil {
			_ = m.runtime.ReloadRoutes(context.Background())
			m.runtime.InvalidateContent()
		}
	}(job)

	return job, nil
}

// Get returns a job by ID or nil.
func (m *WordPressImportManager) Get(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

// Cancel removes the job and cleans temp file. Cannot cancel an active import.
func (m *WordPressImportManager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("job not found")
	}
	if m.activeImportID == id && !job.Done {
		return fmt.Errorf("cannot cancel running import")
	}
	if job.TempPath != "" {
		_ = os.Remove(job.TempPath)
	}
	delete(m.jobs, id)
	return nil
}
