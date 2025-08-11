package external

import (
	"context"
	"fmt"
	"time"

	"github.com/personal/home-work-ad-process/internal/application/service"
	"github.com/personal/home-work-ad-process/internal/domain/command"
)

// MockCommandExecutor implements command.Executor for testing/demo purposes
type MockCommandExecutor struct {
	adService *service.AdService
}

// NewMockCommandExecutor creates a new mock command executor
func NewMockCommandExecutor(adService *service.AdService) *MockCommandExecutor {
	return &MockCommandExecutor{
		adService: adService,
	}
}

// Execute executes a validated command
func (e *MockCommandExecutor) Execute(ctx context.Context, cmd *command.Command) error {
	switch cmd.Intent() {
	case "change_priority_by_game_family":
		return e.executePriorityChangeByGameFamily(ctx, cmd)
		
	case "change_priority_by_age":
		return e.executePriorityChangeByAge(ctx, cmd)
		
	case "show_next_ads":
		return e.executeShowNextAds(ctx, cmd)
		
	case "queue_distribution":
		return e.executeQueueDistribution(ctx, cmd)
		
	case "waiting_ads":
		return e.executeWaitingAds(ctx, cmd)
		
	case "enable_starvation_mode":
		return e.executeEnableStarvationMode(ctx, cmd)
		
	case "set_max_wait_time":
		return e.executeSetMaxWaitTime(ctx, cmd)
		
	default:
		err := fmt.Errorf("unsupported command intent: %s", cmd.Intent())
		cmd.FailExecution(err.Error())
		return err
	}
}

// CanExecute checks if a command can be executed
func (e *MockCommandExecutor) CanExecute(ctx context.Context, cmd *command.Command) bool {
	supportedIntents := map[string]bool{
		"change_priority_by_game_family": true,
		"change_priority_by_age":         true,
		"show_next_ads":                 true,
		"queue_distribution":            true,
		"waiting_ads":                   true,
		"enable_starvation_mode":        true,
		"set_max_wait_time":             true,
	}
	
	return supportedIntents[cmd.Intent()]
}

func (e *MockCommandExecutor) executePriorityChangeByGameFamily(ctx context.Context, cmd *command.Command) error {
	priority, _ := cmd.GetPriorityParameter("priority")
	gameFamily, _ := cmd.GetStringParameter("gameFamily")
	
	count, err := e.adService.ChangePriorityForGameFamily(ctx, gameFamily, priority)
	if err != nil {
		cmd.FailExecution(err.Error())
		return err
	}
	
	result := map[string]interface{}{
		"adsModified":   count,
		"gameFamily":    gameFamily,
		"newPriority":   int(priority),
		"message":       fmt.Sprintf("Updated priority to %d for %d ads in %s family", priority, count, gameFamily),
	}
	
	cmd.CompleteExecution(result)
	return nil
}

func (e *MockCommandExecutor) executePriorityChangeByAge(ctx context.Context, cmd *command.Command) error {
	priority, _ := cmd.GetPriorityParameter("priority")
	minutes, _ := cmd.GetIntParameter("minutes")
	
	duration := time.Duration(minutes) * time.Minute
	count, err := e.adService.ChangePriorityForOlderAds(ctx, duration, priority)
	if err != nil {
		cmd.FailExecution(err.Error())
		return err
	}
	
	result := map[string]interface{}{
		"adsModified":  count,
		"newPriority":  int(priority),
		"olderThan":    fmt.Sprintf("%d minutes", minutes),
		"message":      fmt.Sprintf("Updated priority to %d for %d ads older than %d minutes", priority, count, minutes),
	}
	
	cmd.CompleteExecution(result)
	return nil
}

func (e *MockCommandExecutor) executeShowNextAds(ctx context.Context, cmd *command.Command) error {
	count, _ := cmd.GetIntParameter("count")
	
	nextAds, err := e.adService.GetNextAds(ctx, count)
	if err != nil {
		cmd.FailExecution(err.Error())
		return err
	}
	
	// Convert queue items to a more readable format
	adsList := make([]map[string]interface{}, len(nextAds))
	for i, item := range nextAds {
		adsList[i] = map[string]interface{}{
			"adId":     item.AdID().String(),
			"priority": int(item.Priority()),
			"position": i + 1,
		}
	}
	
	result := map[string]interface{}{
		"nextAds": adsList,
		"count":   len(nextAds),
		"message": fmt.Sprintf("Next %d ads to be processed", len(nextAds)),
	}
	
	cmd.CompleteExecution(result)
	return nil
}

func (e *MockCommandExecutor) executeQueueDistribution(ctx context.Context, cmd *command.Command) error {
	distribution, err := e.adService.GetQueueDistribution(ctx)
	if err != nil {
		cmd.FailExecution(err.Error())
		return err
	}
	
	// Convert to more readable format
	distributionMap := make(map[string]interface{})
	var total int64
	
	for priority, count := range distribution {
		key := fmt.Sprintf("priority_%d", priority)
		distributionMap[key] = count
		total += count
	}
	
	result := map[string]interface{}{
		"distribution": distributionMap,
		"total":        total,
		"message":      fmt.Sprintf("Current queue has %d ads across all priorities", total),
	}
	
	cmd.CompleteExecution(result)
	return nil
}

func (e *MockCommandExecutor) executeWaitingAds(ctx context.Context, cmd *command.Command) error {
	minutes, _ := cmd.GetIntParameter("minutes")
	
	waitTime := time.Duration(minutes) * time.Minute
	waitingAds, err := e.adService.GetWaitingAds(ctx, waitTime)
	if err != nil {
		cmd.FailExecution(err.Error())
		return err
	}
	
	// Convert to readable format
	adsList := make([]map[string]interface{}, len(waitingAds))
	for i, adEntity := range waitingAds {
		adsList[i] = map[string]interface{}{
			"adId":       adEntity.ID().String(),
			"title":      adEntity.Title(),
			"gameFamily": adEntity.GameFamily(),
			"priority":   int(adEntity.Priority()),
			"waitTime":   adEntity.WaitTime().String(),
			"status":     string(adEntity.Status()),
		}
	}
	
	result := map[string]interface{}{
		"waitingAds":  adsList,
		"count":       len(waitingAds),
		"waitTimeMin": minutes,
		"message":     fmt.Sprintf("Found %d ads waiting longer than %d minutes", len(waitingAds), minutes),
	}
	
	cmd.CompleteExecution(result)
	return nil
}

func (e *MockCommandExecutor) executeEnableStarvationMode(ctx context.Context, cmd *command.Command) error {
	// This would typically update queue configuration
	// For now, we'll just return a success message
	
	result := map[string]interface{}{
		"starvationMode": "enabled",
		"message":        "Anti-starvation mechanism has been disabled (starvation mode enabled)",
		"warning":        "Low priority ads may now wait indefinitely",
	}
	
	cmd.CompleteExecution(result)
	return nil
}

func (e *MockCommandExecutor) executeSetMaxWaitTime(ctx context.Context, cmd *command.Command) error {
	seconds, _ := cmd.GetIntParameter("seconds")
	
	// This would typically update queue configuration
	// For now, we'll just return a success message
	
	result := map[string]interface{}{
		"maxWaitTimeSeconds": seconds,
		"maxWaitTime":        fmt.Sprintf("%d seconds", seconds),
		"message":            fmt.Sprintf("Maximum wait time updated to %d seconds", seconds),
	}
	
	cmd.CompleteExecution(result)
	return nil
}