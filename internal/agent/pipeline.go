package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/data"
	"slate/internal/events"
)

// RunPipeline executes a pipeline and returns the final output string.
//
// Steps marked "sequential" run in order, each receiving the previous step's output.
// Consecutive steps marked "parallel" form a group that runs concurrently; their
// outputs are joined with "\n---\n" and passed to the next group.
func (r *Runner) RunPipeline(ctx context.Context, pipelineID ksuid.KSUID, input string) (string, error) {
	pipeline, err := r.store.GetPipeline(pipelineID)
	if err != nil {
		return "", fmt.Errorf("loading pipeline: %w", err)
	}

	wsStr := pipeline.WorkspaceID.String()
	pidStr := pipelineID.String()
	start := time.Now()

	r.emitEvent(events.Event{
		WorkspaceID: wsStr,
		Type:        events.EventPipelineStarted,
		PipelineID:  pidStr,
	})

	if len(pipeline.Steps) == 0 {
		r.emitEvent(events.Event{
			WorkspaceID: wsStr,
			Type:        events.EventPipelineCompleted,
			PipelineID:  pidStr,
			LatencyMs:   time.Since(start).Milliseconds(),
		})
		return input, nil
	}

	// Emit a completed or failed event when the function returns.
	var runErr error
	defer func() {
		evType := events.EventPipelineCompleted
		errStr := ""
		if runErr != nil {
			evType = events.EventPipelineFailed
			errStr = runErr.Error()
		}
		r.emitEvent(events.Event{
			WorkspaceID: wsStr,
			Type:        evType,
			PipelineID:  pidStr,
			LatencyMs:   time.Since(start).Milliseconds(),
			Error:       errStr,
		})
	}()

	current := input
	i := 0
	for i < len(pipeline.Steps) {
		step := pipeline.Steps[i]

		if step.Mode == data.StepModeParallel {
			// Collect all consecutive parallel steps into a group.
			var group []data.PipelineStep
			for i < len(pipeline.Steps) && pipeline.Steps[i].Mode == data.StepModeParallel {
				group = append(group, pipeline.Steps[i])
				i++
			}

			results := make([]string, len(group))
			errs := make([]error, len(group))
			var wg sync.WaitGroup
			for j, s := range group {
				wg.Add(1)
				go func(idx int, agentID ksuid.KSUID) {
					defer wg.Done()
					result, err := r.Run(ctx, agentID, current, nil)
					if err != nil {
						errs[idx] = err
						return
					}
					results[idx] = result.Response
				}(j, s.AgentID)
			}
			wg.Wait()

			for _, err := range errs {
				if err != nil {
					runErr = err
					return "", runErr
				}
			}
			current = strings.Join(results, "\n---\n")
		} else {
			// Sequential: run and advance output.
			result, err := r.Run(ctx, step.AgentID, current, nil)
			if err != nil {
				runErr = fmt.Errorf("pipeline step %d: %w", i, err)
				return "", runErr
			}
			current = result.Response
			i++
		}
	}

	return current, nil
}
