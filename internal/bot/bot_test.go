package bot

import (
	"reflect"
	"testing"
	"time"
)

func TestParseDeadline(t *testing.T) {
	// Create a bot instance (db can be nil for these tests)
	b := &Bot{}

	// Get current time for reference
	now := time.Now()
	berlin, _ := time.LoadLocation("Europe/Berlin")
	nowInBerlin := now.In(berlin)

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		checkResult func(time.Time) bool
		description string
	}{
		{
			name:    "empty input - default 15 minutes",
			input:   "",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				expected := now.Add(15 * time.Minute)
				diff := result.Sub(expected).Abs()
				return diff < 2*time.Second // Allow 2 second tolerance
			},
			description: "Should default to 15 minutes from now",
		},
		{
			name:    "duration 30min",
			input:   "30min",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				expected := now.Add(30 * time.Minute)
				diff := result.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			description: "Should add 30 minutes from now",
		},
		{
			name:    "duration 1h",
			input:   "1h",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				expected := now.Add(1 * time.Hour)
				diff := result.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			description: "Should add 1 hour from now",
		},
		{
			name:    "duration 2h",
			input:   "2h",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				expected := now.Add(2 * time.Hour)
				diff := result.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			description: "Should add 2 hours from now",
		},
		{
			name:    "duration 45min",
			input:   "45min",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				expected := now.Add(45 * time.Minute)
				diff := result.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			description: "Should add 45 minutes from now",
		},
		{
			name:    "time format HH:MM - future time today",
			input:   "23:59",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				resultInBerlin := result.In(berlin)
				// Should be today at 23:59 or tomorrow at 23:59 in Berlin time
				return resultInBerlin.Hour() == 23 && resultInBerlin.Minute() == 59
			},
			description: "Should parse to 23:59 in Berlin time",
		},
		{
			name:    "time format HH:MM - morning time",
			input:   "09:30",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				resultInBerlin := result.In(berlin)
				return resultInBerlin.Hour() == 9 && resultInBerlin.Minute() == 30
			},
			description: "Should parse to 09:30 in Berlin time",
		},
		{
			name:    "time format HH:MM - noon",
			input:   "12:00",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				resultInBerlin := result.In(berlin)
				return resultInBerlin.Hour() == 12 && resultInBerlin.Minute() == 0
			},
			description: "Should parse to 12:00 in Berlin time",
		},
		{
			name:    "time format H:MM - single digit hour",
			input:   "9:30",
			wantErr: false,
			checkResult: func(result time.Time) bool {
				resultInBerlin := result.In(berlin)
				return resultInBerlin.Hour() == 9 && resultInBerlin.Minute() == 30
			},
			description: "Should parse single digit hour",
		},
		{
			name:        "invalid format",
			input:       "invalid",
			wantErr:     true,
			checkResult: nil,
			description: "Should return error for invalid format",
		},
		{
			name:        "invalid time format",
			input:       "25:00",
			wantErr:     true,
			checkResult: nil,
			description: "Should return error for invalid hour",
		},
		{
			name:        "invalid duration format",
			input:       "30mins", // wrong format
			wantErr:     true,
			checkResult: nil,
			description: "Should return error for wrong duration format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := b.parseDeadline(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseDeadline() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkResult != nil && !tt.checkResult(result) {
				t.Errorf("parseDeadline() result check failed: %s\nInput: %s\nResult: %v (Berlin: %v)\nNow: %v (Berlin: %v)",
					tt.description, tt.input, result, result.In(berlin), now, nowInBerlin)
			}
		})
	}
}

func TestParseGyroskopArgs(t *testing.T) {
	b := &Bot{}
	now := time.Now()

	tests := []struct {
		name            string
		args            string
		wantName        string
		wantOptions     []string
		wantDeadlineChk func(time.Time) bool
		wantErr         bool
		description     string
	}{
		{
			name:        "empty args - all defaults",
			args:        "",
			wantName:    "Gyros",
			wantOptions: []string{"Fleisch", "Vegetarisch"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(15 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should use all defaults",
		},
		{
			name:        "only time - 30min",
			args:        "30min",
			wantName:    "Gyros",
			wantOptions: []string{"Fleisch", "Vegetarisch"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(30 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should use 30min deadline with defaults",
		},
		{
			name:        "only time - HH:MM format",
			args:        "17:00",
			wantName:    "Gyros",
			wantOptions: []string{"Fleisch", "Vegetarisch"},
			wantDeadlineChk: func(deadline time.Time) bool {
				berlin, _ := time.LoadLocation("Europe/Berlin")
				deadlineInBerlin := deadline.In(berlin)
				return deadlineInBerlin.Hour() == 17 && deadlineInBerlin.Minute() == 0
			},
			wantErr:     false,
			description: "Should use 17:00 deadline with defaults",
		},
		{
			name:        "name and options only",
			args:        "Pizza, Margherita, Salami, Hawaii",
			wantName:    "Pizza",
			wantOptions: []string{"Margherita", "Salami", "Hawaii"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(15 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should use default deadline with custom name and options",
		},
		{
			name:        "time, name, and options",
			args:        "30min, Burger, Beef, Chicken, Veggie",
			wantName:    "Burger",
			wantOptions: []string{"Beef", "Chicken", "Veggie"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(30 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should parse all components",
		},
		{
			name:        "HH:MM time, name, and options",
			args:        "18:30, Sushi, Salmon, Tuna, Veggie",
			wantName:    "Sushi",
			wantOptions: []string{"Salmon", "Tuna", "Veggie"},
			wantDeadlineChk: func(deadline time.Time) bool {
				berlin, _ := time.LoadLocation("Europe/Berlin")
				deadlineInBerlin := deadline.In(berlin)
				return deadlineInBerlin.Hour() == 18 && deadlineInBerlin.Minute() == 30
			},
			wantErr:     false,
			description: "Should parse HH:MM with name and options",
		},
		{
			name:        "single option",
			args:        "Döner, Classic",
			wantName:    "Döner",
			wantOptions: []string{"Classic"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(15 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should handle single option",
		},
		{
			name:        "extra spaces",
			args:        "  30min  ,  Pizza  ,  Margherita  ,  Salami  ",
			wantName:    "Pizza",
			wantOptions: []string{"Margherita", "Salami"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(30 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should handle extra spaces",
		},
		{
			name:        "2 hour duration",
			args:        "2h",
			wantName:    "Gyros",
			wantOptions: []string{"Fleisch", "Vegetarisch"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(2 * time.Hour)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should handle hour duration",
		},
		{
			name:        "Döner with 10min deadline - comma-separated",
			args:        "10min, Döner, Fleisch, Vegetarisch, Dürüm",
			wantName:    "Döner",
			wantOptions: []string{"Fleisch", "Vegetarisch", "Dürüm"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(10 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should parse '10min, Döner, Fleisch, Vegetarisch, Dürüm' with comma-separated format",
		},
		{
			name:        "Multiple options without time",
			args:        "Döner, Fleisch, Vegetarisch, Dürüm",
			wantName:    "Döner",
			wantOptions: []string{"Fleisch", "Vegetarisch", "Dürüm"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(15 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should use default deadline when time not specified",
		},
		{
			name:        "Time format with multiple options - comma-separated",
			args:        "12:30, Döner, Fleisch, Vegetarisch, Dürüm",
			wantName:    "Döner",
			wantOptions: []string{"Fleisch", "Vegetarisch", "Dürüm"},
			wantDeadlineChk: func(deadline time.Time) bool {
				berlin, _ := time.LoadLocation("Europe/Berlin")
				deadlineInBerlin := deadline.In(berlin)
				return deadlineInBerlin.Hour() == 12 && deadlineInBerlin.Minute() == 30
			},
			wantErr:     false,
			description: "Should parse HH:MM format with multiple options (comma-separated)",
		},
		{
			name:        "5min duration",
			args:        "5min",
			wantName:    "Gyros",
			wantOptions: []string{"Fleisch", "Vegetarisch"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(5 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should handle 5 minute duration",
		},
		{
			name:        "Many food options - comma-separated",
			args:        "15min, Pizza, Margherita, Salami, Hawaiian, Quattro Formaggi, Tonno",
			wantName:    "Pizza",
			wantOptions: []string{"Margherita", "Salami", "Hawaiian", "Quattro Formaggi", "Tonno"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(15 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should handle many food options (comma-separated)",
		},
		{
			name:        "Unicode characters in food name and options",
			args:        "20min, Türkischer Döner, Mit Käse, Ohne Käse, Scharf",
			wantName:    "Türkischer Döner",
			wantOptions: []string{"Mit Käse", "Ohne Käse", "Scharf"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(20 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should handle Unicode characters properly (comma-separated)",
		},
		{
			name:        "Mixed spacing with commas",
			args:        "  10min  ,  Döner  ,  Fleisch  ,  Vegetarisch  ,  Dürüm  ",
			wantName:    "Döner",
			wantOptions: []string{"Fleisch", "Vegetarisch", "Dürüm"},
			wantDeadlineChk: func(deadline time.Time) bool {
				expected := now.Add(10 * time.Minute)
				diff := deadline.Sub(expected).Abs()
				return diff < 2*time.Second
			},
			wantErr:     false,
			description: "Should handle irregular spacing around commas",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deadline, name, options, err := b.parseGyroskopArgs(tt.args)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseGyroskopArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if name != tt.wantName {
				t.Errorf("parseGyroskopArgs() name = %v, want %v", name, tt.wantName)
			}

			if !reflect.DeepEqual(options, tt.wantOptions) {
				t.Errorf("parseGyroskopArgs() options = %v, want %v", options, tt.wantOptions)
			}

			if tt.wantDeadlineChk != nil && !tt.wantDeadlineChk(deadline) {
				t.Errorf("parseGyroskopArgs() deadline check failed: %s\nArgs: %s\nDeadline: %v",
					tt.description, tt.args, deadline)
			}
		})
	}
}

func TestParseOrderText(t *testing.T) {
	b := &Bot{}

	tests := []struct {
		name        string
		text        string
		foodOptions []string
		want        map[string]int
		description string
	}{
		{
			name:        "single order - exact match",
			text:        "2 fleisch",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2},
			description: "Should parse single order with exact match",
		},
		{
			name:        "single order - fuzzy match",
			text:        "3 veg",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Vegetarisch": 3},
			description: "Should parse single order with fuzzy match",
		},
		{
			name:        "multiple orders - comma separated",
			text:        "2 fleisch, 3 veg",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2, "Vegetarisch": 3},
			description: "Should parse multiple orders in one line",
		},
		{
			name:        "multiple orders - newline separated",
			text:        "2 fleisch\n3 veg",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2, "Vegetarisch": 3},
			description: "Should parse multiple orders on separate lines",
		},
		{
			name:        "prefix match",
			text:        "2 fl",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2},
			description: "Should handle prefix matching",
		},
		{
			name:        "pizza options",
			text:        "2 marg, 1 sal, 3 hawaii",
			foodOptions: []string{"Margherita", "Salami", "Hawaiian"},
			want:        map[string]int{"Margherita": 2, "Salami": 1, "Hawaiian": 3},
			description: "Should handle pizza options with fuzzy matching",
		},
		{
			name:        "no space between number and text",
			text:        "2fleisch",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2},
			description: "Should handle no space format",
		},
		{
			name:        "extra spaces",
			text:        "  2   fleisch  ",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2},
			description: "Should handle extra spaces",
		},
		{
			name:        "overwrite previous quantity",
			text:        "2 fleisch, 5 fleisch",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 5},
			description: "Should overwrite with last quantity for same option",
		},
		{
			name:        "invalid quantity - too high",
			text:        "15 fleisch",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        nil,
			description: "Should reject quantity > 10",
		},
		{
			name:        "invalid quantity - negative",
			text:        "-2 fleisch",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        nil,
			description: "Should reject negative quantity",
		},
		{
			name:        "no match found",
			text:        "2 xyz",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        nil,
			description: "Should return nil for no match",
		},
		{
			name:        "invalid format - no number",
			text:        "fleisch",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        nil,
			description: "Should return nil for missing quantity",
		},
		{
			name:        "mixed valid and invalid",
			text:        "2 fleisch, xyz",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2},
			description: "Should parse valid parts and ignore invalid",
		},
		{
			name:        "empty text",
			text:        "",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        nil,
			description: "Should return nil for empty text",
		},
		{
			name:        "only whitespace",
			text:        "   \n  \n  ",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        nil,
			description: "Should return nil for whitespace only",
		},
		{
			name:        "case insensitive",
			text:        "2 FLEISCH",
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        map[string]int{"Fleisch": 2},
			description: "Should be case insensitive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.parseOrderText(tt.text, tt.foodOptions)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseOrderText() = %v, want %v\nDescription: %s",
					got, tt.want, tt.description)
			}
		})
	}
}

func TestFormatOrderQuantities(t *testing.T) {
	b := &Bot{}

	tests := []struct {
		name        string
		quantities  map[string]int
		foodOptions []string
		want        string
	}{
		{
			name:        "single option",
			quantities:  map[string]int{"Fleisch": 2},
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        "2 Fleisch",
		},
		{
			name:        "single option - quantity 1",
			quantities:  map[string]int{"Fleisch": 1},
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        "1 Fleisch",
		},
		{
			name:        "multiple options",
			quantities:  map[string]int{"Fleisch": 2, "Vegetarisch": 3},
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        "2 Fleisch, 3 Vegetarisch",
		},
		{
			name:        "maintains order from foodOptions",
			quantities:  map[string]int{"Vegetarisch": 3, "Fleisch": 2},
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        "2 Fleisch, 3 Vegetarisch",
		},
		{
			name:        "ignores zero quantities",
			quantities:  map[string]int{"Fleisch": 2, "Vegetarisch": 0},
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        "2 Fleisch",
		},
		{
			name:        "empty quantities",
			quantities:  map[string]int{},
			foodOptions: []string{"Fleisch", "Vegetarisch"},
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.formatOrderQuantities(tt.quantities, tt.foodOptions)
			if got != tt.want {
				t.Errorf("formatOrderQuantities() = %v, want %v", got, tt.want)
			}
		})
	}
}
