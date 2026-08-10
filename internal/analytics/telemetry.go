package analytics

import (
	"context"
	"log"
	"time"

	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
)

type TelemetryLogger struct {
	pg       *db.PostgresDB
	logQueue chan model.UsageLog
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewTelemetryLogger(pg *db.PostgresDB, bufferSize int) *TelemetryLogger {
	ctx, cancel := context.WithCancel(context.Background())
	logger := &TelemetryLogger{
		pg:       pg,
		logQueue: make(chan model.UsageLog, bufferSize),
		ctx:      ctx,
		cancel:   cancel,
	}

	go logger.startWorkerPool(2, 50, 1*time.Second)
	return logger
}

func (t *TelemetryLogger) Log(logEntry model.UsageLog) {
	select {
	case t.logQueue <- logEntry:
	default:
		log.Printf("Warning: Telemetry log queue is full, dropping log for key: %s", logEntry.KeyID)
	}
}

func (t *TelemetryLogger) startWorkerPool(workers int, batchSize int, flushInterval time.Duration) {
	for i := 0; i < workers; i++ {
		go func() {
			var batch []model.UsageLog
			ticker := time.NewTicker(flushInterval)
			defer ticker.Stop()

			for {
				select {
				case <-t.ctx.Done():
					if len(batch) > 0 && t.pg != nil {
						_ = t.pg.InsertUsageLogsBatch(batch)
					}
					return

				case logEntry, ok := <-t.logQueue:
					if !ok {
						if len(batch) > 0 && t.pg != nil {
							_ = t.pg.InsertUsageLogsBatch(batch)
						}
						return
					}
					batch = append(batch, logEntry)
					if len(batch) >= batchSize {
						if t.pg != nil {
							if err := t.pg.InsertUsageLogsBatch(batch); err != nil {
								log.Printf("Error inserting telemetry batch: %v", err)
							}
						}
						batch = make([]model.UsageLog, 0, batchSize)
					}

				case <-ticker.C:
					if len(batch) > 0 {
						if t.pg != nil {
							if err := t.pg.InsertUsageLogsBatch(batch); err != nil {
								log.Printf("Error inserting telemetry batch on ticker: %v", err)
							}
						}
						batch = make([]model.UsageLog, 0, batchSize)
					}
				}
			}
		}()
	}
}

func (t *TelemetryLogger) Close() {
	t.cancel()
	close(t.logQueue)
}
