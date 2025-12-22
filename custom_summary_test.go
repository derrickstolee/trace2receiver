package trace2receiver

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test helper functions
var x_css_path string = "TEST/custom_summary.yml"

func x_TryLoadCustomSummarySettings(t *testing.T, yml string, path string) *CustomSummarySettings {
	css, err := parseCustomSummarySettingsFromBuffer([]byte(yml), path)
	if err != nil {
		t.Fatalf("parseCustomSummarySettings(%s): %s", path, err.Error())
	}
	return css
}

// ////////////////////////////////////////////////////////
// Configuration Parsing Tests
// ////////////////////////////////////////////////////////

var x_css_valid_yml string = `
message_patterns:
  - prefix: "test:"
    field_name: "testCount"
  - prefix: "prefix:"
    field_name: "prefixCount"
region_timers:
  - category: "cat1"
    label: "label1"
    count_field: "count1"
    time_field: "time1"
  - category: "cat2"
    label: "label2"
    count_field: "count2"
`

func Test_ValidCustomSummarySettings(t *testing.T) {
	css := x_TryLoadCustomSummarySettings(t, x_css_valid_yml, x_css_path)

	assert.NotNil(t, css)
	assert.Equal(t, 2, len(css.MessagePatterns))
	assert.Equal(t, "test:", css.MessagePatterns[0].Prefix)
	assert.Equal(t, "testCount", css.MessagePatterns[0].FieldName)

	assert.Equal(t, 2, len(css.RegionTimers))
	assert.Equal(t, "cat1", css.RegionTimers[0].Category)
	assert.Equal(t, "label1", css.RegionTimers[0].Label)
	assert.Equal(t, "count1", css.RegionTimers[0].CountField)
	assert.Equal(t, "time1", css.RegionTimers[0].TimeField)
}

var x_css_empty_prefix_yml string = `
message_patterns:
  - prefix: ""
    field_name: "testCount"
`

func Test_EmptyPrefix_Rejected(t *testing.T) {
	_, err := parseCustomSummarySettingsFromBuffer([]byte(x_css_empty_prefix_yml), x_css_path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prefix cannot be empty")
}

var x_css_empty_field_name_yml string = `
message_patterns:
  - prefix: "test:"
    field_name: ""
`

func Test_EmptyFieldName_Rejected(t *testing.T) {
	_, err := parseCustomSummarySettingsFromBuffer([]byte(x_css_empty_field_name_yml), x_css_path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "field_name cannot be empty")
}

var x_css_duplicate_field_yml string = `
message_patterns:
  - prefix: "test1:"
    field_name: "count"
  - prefix: "test2:"
    field_name: "count"
`

func Test_DuplicateFieldNames_Rejected(t *testing.T) {
	_, err := parseCustomSummarySettingsFromBuffer([]byte(x_css_duplicate_field_yml), x_css_path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate field_name")
}

var x_css_empty_category_yml string = `
region_timers:
  - category: ""
    label: "label1"
    count_field: "count1"
`

func Test_EmptyCategory_Rejected(t *testing.T) {
	_, err := parseCustomSummarySettingsFromBuffer([]byte(x_css_empty_category_yml), x_css_path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "category cannot be empty")
}

var x_css_no_fields_yml string = `
region_timers:
  - category: "cat1"
    label: "label1"
`

func Test_NoCountOrTimeField_Rejected(t *testing.T) {
	_, err := parseCustomSummarySettingsFromBuffer([]byte(x_css_no_fields_yml), x_css_path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of count_field or time_field must be specified")
}

var x_css_duplicate_cross_type_yml string = `
message_patterns:
  - prefix: "test:"
    field_name: "myCount"
region_timers:
  - category: "cat1"
    label: "label1"
    count_field: "myCount"
`

func Test_DuplicateFieldNames_CrossType_Rejected(t *testing.T) {
	_, err := parseCustomSummarySettingsFromBuffer([]byte(x_css_duplicate_cross_type_yml), x_css_path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate field_name")
}

// ////////////////////////////////////////////////////////
// Message Pattern Matching Tests
// ////////////////////////////////////////////////////////

func Test_MessagePatternMatching_Basic(t *testing.T) {
	css := &CustomSummarySettings{
		MessagePatterns: []MessagePatternRule{
			{Prefix: "gh_client__queue:", FieldName: "queuedCount"},
			{Prefix: "gh_client__immediate:", FieldName: "immediateCount"},
		},
	}

	csa := newCustomSummaryAccumulator()

	// Simulate a fake tr2 dataset with the config
	tr2 := &trace2Dataset{
		process: TrProcess{
			customSummary: csa,
		},
	}
	tr2.rcvr_base = &Rcvr_Base{
		RcvrConfig: &Config{
			customSummary: css,
		},
	}

	// Test matching messages
	apply__custom_summary_message(tr2, "gh_client__queue:abc123")
	apply__custom_summary_message(tr2, "gh_client__queue:def456")
	apply__custom_summary_message(tr2, "gh_client__immediate:xyz789")
	apply__custom_summary_message(tr2, "other_message")

	summaryMap := csa.toMap()
	assert.Equal(t, int64(2), summaryMap["queuedCount"])
	assert.Equal(t, int64(1), summaryMap["immediateCount"])
	_, exists := summaryMap["other"]
	assert.False(t, exists)
}

func Test_MessagePatternMatching_MultipleMatches(t *testing.T) {
	css := &CustomSummarySettings{
		MessagePatterns: []MessagePatternRule{
			{Prefix: "test:", FieldName: "testCount"},
		},
	}

	csa := newCustomSummaryAccumulator()
	tr2 := &trace2Dataset{
		process: TrProcess{
			customSummary: csa,
		},
	}
	tr2.rcvr_base = &Rcvr_Base{
		RcvrConfig: &Config{
			customSummary: css,
		},
	}

	// Multiple matches accumulate
	for i := 0; i < 10; i++ {
		apply__custom_summary_message(tr2, "test:message")
	}

	summaryMap := csa.toMap()
	assert.Equal(t, int64(10), summaryMap["testCount"])
}

func Test_MessagePatternMatching_NoConfig(t *testing.T) {
	// When no custom summary config, should not crash
	tr2 := &trace2Dataset{
		process: TrProcess{
			customSummary: nil,
		},
	}
	tr2.rcvr_base = &Rcvr_Base{
		RcvrConfig: &Config{
			customSummary: nil,
		},
	}

	// Should not crash
	apply__custom_summary_message(tr2, "test:message")
}

// ////////////////////////////////////////////////////////
// Region Timer Aggregation Tests
// ////////////////////////////////////////////////////////

func Test_RegionTimerAggregation_Basic(t *testing.T) {
	css := &CustomSummarySettings{
		RegionTimers: []RegionTimerRule{
			{
				Category:   "gh-client",
				Label:      "objects/prefetch",
				CountField: "prefetchCount",
				TimeField:  "prefetchTime",
			},
		},
	}

	csa := newCustomSummaryAccumulator()
	tr2 := &trace2Dataset{
		process: TrProcess{
			customSummary: csa,
		},
	}
	tr2.rcvr_base = &Rcvr_Base{
		RcvrConfig: &Config{
			customSummary: css,
		},
	}

	// Create some test regions
	r1 := &TrRegion{
		category: "gh-client",
		label:    "objects/prefetch",
	}
	// Simulate 10 seconds duration
	r1.lifetime.startTime = mustParseTime(t, "2024-01-01T10:00:00Z")
	r1.lifetime.endTime = mustParseTime(t, "2024-01-01T10:00:10Z")

	r2 := &TrRegion{
		category: "gh-client",
		label:    "objects/prefetch",
	}
	// Simulate 5 seconds duration
	r2.lifetime.startTime = mustParseTime(t, "2024-01-01T10:00:15Z")
	r2.lifetime.endTime = mustParseTime(t, "2024-01-01T10:00:20Z")

	apply__custom_summary_region(tr2, r1)
	apply__custom_summary_region(tr2, r2)

	summaryMap := csa.toMap()
	assert.Equal(t, int64(2), summaryMap["prefetchCount"])
	assert.InDelta(t, 15.0, summaryMap["prefetchTime"], 0.1)
}

func Test_RegionTimerAggregation_CountOnly(t *testing.T) {
	css := &CustomSummarySettings{
		RegionTimers: []RegionTimerRule{
			{
				Category:   "test-cat",
				Label:      "test-label",
				CountField: "testCount",
				TimeField:  "", // No time field
			},
		},
	}

	csa := newCustomSummaryAccumulator()
	tr2 := &trace2Dataset{
		process: TrProcess{
			customSummary: csa,
		},
	}
	tr2.rcvr_base = &Rcvr_Base{
		RcvrConfig: &Config{
			customSummary: css,
		},
	}

	r := &TrRegion{
		category: "test-cat",
		label:    "test-label",
	}
	r.lifetime.startTime = mustParseTime(t, "2024-01-01T10:00:00Z")
	r.lifetime.endTime = mustParseTime(t, "2024-01-01T10:00:10Z")

	apply__custom_summary_region(tr2, r)

	summaryMap := csa.toMap()
	assert.Equal(t, int64(1), summaryMap["testCount"])
	_, hasTime := summaryMap["testTime"]
	assert.False(t, hasTime)
}

func Test_RegionTimerAggregation_TimeOnly(t *testing.T) {
	css := &CustomSummarySettings{
		RegionTimers: []RegionTimerRule{
			{
				Category:   "test-cat",
				Label:      "test-label",
				CountField: "", // No count field
				TimeField:  "testTime",
			},
		},
	}

	csa := newCustomSummaryAccumulator()
	tr2 := &trace2Dataset{
		process: TrProcess{
			customSummary: csa,
		},
	}
	tr2.rcvr_base = &Rcvr_Base{
		RcvrConfig: &Config{
			customSummary: css,
		},
	}

	r := &TrRegion{
		category: "test-cat",
		label:    "test-label",
	}
	r.lifetime.startTime = mustParseTime(t, "2024-01-01T10:00:00Z")
	r.lifetime.endTime = mustParseTime(t, "2024-01-01T10:00:10Z")

	apply__custom_summary_region(tr2, r)

	summaryMap := csa.toMap()
	_, hasCount := summaryMap["testCount"]
	assert.False(t, hasCount)
	assert.InDelta(t, 10.0, summaryMap["testTime"], 0.1)
}

func Test_RegionTimerAggregation_NoMatch(t *testing.T) {
	css := &CustomSummarySettings{
		RegionTimers: []RegionTimerRule{
			{
				Category:   "gh-client",
				Label:      "objects/prefetch",
				CountField: "prefetchCount",
			},
		},
	}

	csa := newCustomSummaryAccumulator()
	tr2 := &trace2Dataset{
		process: TrProcess{
			customSummary: csa,
		},
	}
	tr2.rcvr_base = &Rcvr_Base{
		RcvrConfig: &Config{
			customSummary: css,
		},
	}

	// Different category/label - should not match
	r := &TrRegion{
		category: "other-cat",
		label:    "other-label",
	}
	r.lifetime.startTime = mustParseTime(t, "2024-01-01T10:00:00Z")
	r.lifetime.endTime = mustParseTime(t, "2024-01-01T10:00:10Z")

	apply__custom_summary_region(tr2, r)

	summaryMap := csa.toMap()
	assert.Equal(t, 0, len(summaryMap))
}

// ////////////////////////////////////////////////////////
// JSON Marshaling Tests
// ////////////////////////////////////////////////////////

func Test_ToMap_MixedValues(t *testing.T) {
	csa := newCustomSummaryAccumulator()

	csa.incrementMessageCount("queuedCount")
	csa.incrementMessageCount("queuedCount")
	csa.addRegionMetrics("prefetchCount", "prefetchTime", 30.5)
	csa.addRegionMetrics("batchCount", "batchTime", 12.3)

	summaryMap := csa.toMap()
	assert.Equal(t, 5, len(summaryMap))
	assert.Equal(t, int64(2), summaryMap["queuedCount"])
	assert.Equal(t, int64(1), summaryMap["prefetchCount"])
	assert.InDelta(t, 30.5, summaryMap["prefetchTime"], 0.01)
	assert.Equal(t, int64(1), summaryMap["batchCount"])
	assert.InDelta(t, 12.3, summaryMap["batchTime"], 0.01)
}

func Test_JSONMarshal_Format(t *testing.T) {
	csa := newCustomSummaryAccumulator()
	csa.incrementMessageCount("queuedCount")
	csa.incrementMessageCount("queuedCount")
	csa.addRegionMetrics("prefetchCount", "prefetchTime", 30.4)

	summaryMap := csa.toMap()
	jsonBytes, err := json.Marshal(summaryMap)
	assert.NoError(t, err)

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), result["queuedCount"])
	assert.Equal(t, float64(1), result["prefetchCount"])
	assert.InDelta(t, 30.4, result["prefetchTime"], 0.01)
}

func Test_ToMap_Empty(t *testing.T) {
	csa := newCustomSummaryAccumulator()
	summaryMap := csa.toMap()
	assert.Equal(t, 0, len(summaryMap))
}

// ////////////////////////////////////////////////////////
// Test helpers
// ////////////////////////////////////////////////////////

func mustParseTime(t *testing.T, timeStr string) time.Time {
	parsed, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		t.Fatalf("Failed to parse time %s: %s", timeStr, err.Error())
	}
	return parsed
}
