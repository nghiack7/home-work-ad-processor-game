package external

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/personal/home-work-ad-process/internal/domain/command"
)

// MockCommandParser implements command.Parser for testing/demo purposes
// In production, this would integrate with Google ADK
type MockCommandParser struct{}

// NewMockCommandParser creates a new mock command parser
func NewMockCommandParser() *MockCommandParser {
	return &MockCommandParser{}
}

// Parse parses natural language text into a structured command
func (p *MockCommandParser) Parse(ctx context.Context, text string) (*command.Command, error) {
	text = strings.TrimSpace(strings.ToLower(text))
	
	// Priority change for game family pattern
	if matched, err := regexp.MatchString(`change priority to \d+ for all ads in the .+ family`, text); err == nil && matched {
		return p.parsePriorityChangeByGameFamily(text)
	}
	
	// Priority change for old ads pattern
	if matched, err := regexp.MatchString(`set priority to \d+ for ads older than \d+ minutes?`, text); err == nil && matched {
		return p.parsePriorityChangeByAge(text)
	}
	
	// Show next ads pattern
	if matched, err := regexp.MatchString(`show the next \d+ ads to be processed`, text); err == nil && matched {
		return p.parseShowNextAds(text)
	}
	
	// Queue distribution pattern
	if matched, err := regexp.MatchString(`what.s the current queue distribution by priority`, text); err == nil && matched {
		return p.parseQueueDistribution(text)
	}
	
	// List waiting ads pattern
	if matched, err := regexp.MatchString(`list all ads waiting longer than \d+ minutes?`, text); err == nil && matched {
		return p.parseWaitingAds(text)
	}
	
	// Enable starvation mode
	if strings.Contains(text, "enable starvation mode") {
		return p.parseEnableStarvationMode(text)
	}
	
	// Set max wait time
	if matched, err := regexp.MatchString(`set maximum wait time to \d+ seconds?`, text); err == nil && matched {
		return p.parseSetMaxWaitTime(text)
	}
	
	return nil, fmt.Errorf("unable to parse command: %s", text)
}

func (p *MockCommandParser) parsePriorityChangeByGameFamily(text string) (*command.Command, error) {
	re := regexp.MustCompile(`change priority to (\d+) for all ads in the (.+) family`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid priority change command")
	}
	
	priority, err := strconv.Atoi(matches[1])
	if err != nil || priority < 1 || priority > 5 {
		return nil, fmt.Errorf("invalid priority value")
	}
	
	gameFamily := strings.TrimSpace(matches[2])
	
	parameters := map[string]interface{}{
		"priority":    priority,
		"gameFamily":  gameFamily,
	}
	
	return command.NewCommand(
		text,
		command.CommandTypeQueueModification,
		"change_priority_by_game_family",
		parameters,
	), nil
}

func (p *MockCommandParser) parsePriorityChangeByAge(text string) (*command.Command, error) {
	re := regexp.MustCompile(`set priority to (\d+) for ads older than (\d+) minutes?`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid priority change command")
	}
	
	priority, err := strconv.Atoi(matches[1])
	if err != nil || priority < 1 || priority > 5 {
		return nil, fmt.Errorf("invalid priority value")
	}
	
	minutes, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid minutes value")
	}
	
	parameters := map[string]interface{}{
		"priority": priority,
		"minutes":  minutes,
	}
	
	return command.NewCommand(
		text,
		command.CommandTypeQueueModification,
		"change_priority_by_age",
		parameters,
	), nil
}

func (p *MockCommandParser) parseShowNextAds(text string) (*command.Command, error) {
	re := regexp.MustCompile(`show the next (\d+) ads to be processed`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 2 {
		return nil, fmt.Errorf("invalid show next ads command")
	}
	
	count, err := strconv.Atoi(matches[1])
	if err != nil || count <= 0 {
		return nil, fmt.Errorf("invalid count value")
	}
	
	parameters := map[string]interface{}{
		"count": count,
	}
	
	return command.NewCommand(
		text,
		command.CommandTypeStatusQuery,
		"show_next_ads",
		parameters,
	), nil
}

func (p *MockCommandParser) parseQueueDistribution(text string) (*command.Command, error) {
	return command.NewCommand(
		text,
		command.CommandTypeAnalytics,
		"queue_distribution",
		map[string]interface{}{},
	), nil
}

func (p *MockCommandParser) parseWaitingAds(text string) (*command.Command, error) {
	re := regexp.MustCompile(`list all ads waiting longer than (\d+) minutes?`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 2 {
		return nil, fmt.Errorf("invalid waiting ads command")
	}
	
	minutes, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minutes value")
	}
	
	parameters := map[string]interface{}{
		"minutes": minutes,
	}
	
	return command.NewCommand(
		text,
		command.CommandTypeStatusQuery,
		"waiting_ads",
		parameters,
	), nil
}

func (p *MockCommandParser) parseEnableStarvationMode(text string) (*command.Command, error) {
	return command.NewCommand(
		text,
		command.CommandTypeSystemConfiguration,
		"enable_starvation_mode",
		map[string]interface{}{},
	), nil
}

func (p *MockCommandParser) parseSetMaxWaitTime(text string) (*command.Command, error) {
	re := regexp.MustCompile(`set maximum wait time to (\d+) seconds?`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 2 {
		return nil, fmt.Errorf("invalid max wait time command")
	}
	
	seconds, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid seconds value")
	}
	
	parameters := map[string]interface{}{
		"seconds": seconds,
	}
	
	return command.NewCommand(
		text,
		command.CommandTypeSystemConfiguration,
		"set_max_wait_time",
		parameters,
	), nil
}

// ValidateCommand validates that a command has all required parameters
func (p *MockCommandParser) ValidateCommand(ctx context.Context, cmd *command.Command) error {
	switch cmd.Intent() {
	case "change_priority_by_game_family":
		if _, exists := cmd.GetPriorityParameter("priority"); !exists {
			return fmt.Errorf("missing or invalid priority parameter")
		}
		if _, exists := cmd.GetStringParameter("gameFamily"); !exists {
			return fmt.Errorf("missing gameFamily parameter")
		}
		
	case "change_priority_by_age":
		if _, exists := cmd.GetPriorityParameter("priority"); !exists {
			return fmt.Errorf("missing or invalid priority parameter")
		}
		if _, exists := cmd.GetIntParameter("minutes"); !exists {
			return fmt.Errorf("missing minutes parameter")
		}
		
	case "show_next_ads":
		if count, exists := cmd.GetIntParameter("count"); !exists || count <= 0 {
			return fmt.Errorf("missing or invalid count parameter")
		}
		
	case "waiting_ads":
		if _, exists := cmd.GetIntParameter("minutes"); !exists {
			return fmt.Errorf("missing minutes parameter")
		}
		
	case "set_max_wait_time":
		if _, exists := cmd.GetIntParameter("seconds"); !exists {
			return fmt.Errorf("missing seconds parameter")
		}
	}
	
	return nil
}