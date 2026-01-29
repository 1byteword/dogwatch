package bubbleup

import (
	"math"
	"testing"
)

func TestChiSquared(t *testing.T) {
	tests := []struct {
		name     string
		observed []float64
		expected []float64
		want     float64
	}{
		{
			name:     "identical distributions",
			observed: []float64{10, 20, 30},
			expected: []float64{10, 20, 30},
			want:     0,
		},
		{
			name:     "different distributions",
			observed: []float64{50, 10, 10},
			expected: []float64{20, 25, 25},
			want:     63, // (50-20)^2/20 + (10-25)^2/25 + (10-25)^2/25 = 45 + 9 + 9 = 63
		},
		{
			name:     "empty arrays",
			observed: []float64{},
			expected: []float64{},
			want:     0,
		},
		{
			name:     "mismatched lengths",
			observed: []float64{10, 20},
			expected: []float64{10},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChiSquared(tt.observed, tt.expected)
			if math.Abs(got-tt.want) > 0.1 {
				t.Errorf("ChiSquared() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChiSquaredFromDistributions(t *testing.T) {
	tests := []struct {
		name           string
		anomalous      map[string]int
		baseline       map[string]int
		anomalousTotal int
		baselineTotal  int
		wantPositive   bool
	}{
		{
			name:           "significant difference",
			anomalous:      map[string]int{"shard-7": 98, "shard-1": 1, "shard-2": 1},
			baseline:       map[string]int{"shard-7": 8, "shard-1": 23, "shard-2": 22, "shard-3": 23, "shard-4": 24},
			anomalousTotal: 100,
			baselineTotal:  100,
			wantPositive:   true,
		},
		{
			name:           "identical distributions",
			anomalous:      map[string]int{"a": 50, "b": 50},
			baseline:       map[string]int{"a": 50, "b": 50},
			anomalousTotal: 100,
			baselineTotal:  100,
			wantPositive:   false,
		},
		{
			name:           "empty anomalous",
			anomalous:      map[string]int{},
			baseline:       map[string]int{"a": 50},
			anomalousTotal: 0,
			baselineTotal:  50,
			wantPositive:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChiSquaredFromDistributions(tt.anomalous, tt.baseline, tt.anomalousTotal, tt.baselineTotal)
			if tt.wantPositive && got <= 0 {
				t.Errorf("ChiSquaredFromDistributions() = %v, want positive", got)
			}
			if !tt.wantPositive && got > 0.1 {
				t.Errorf("ChiSquaredFromDistributions() = %v, want ~0", got)
			}
		})
	}
}

func TestCalculateLift(t *testing.T) {
	tests := []struct {
		name          string
		anomalousRate float64
		baselineRate  float64
		want          float64
	}{
		{
			name:          "2x lift",
			anomalousRate: 0.5,
			baselineRate:  0.25,
			want:          2.0,
		},
		{
			name:          "no lift",
			anomalousRate: 0.25,
			baselineRate:  0.25,
			want:          1.0,
		},
		{
			name:          "zero baseline rate",
			anomalousRate: 0.5,
			baselineRate:  0,
			want:          100.0, // capped at 100
		},
		{
			name:          "both zero",
			anomalousRate: 0,
			baselineRate:  0,
			want:          1.0,
		},
		{
			name:          "very high lift capped",
			anomalousRate: 0.9,
			baselineRate:  0.001,
			want:          100.0, // capped at 100
		},
		{
			name:          "12.25x lift example",
			anomalousRate: 0.98,
			baselineRate:  0.08,
			want:          12.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateLift(tt.anomalousRate, tt.baselineRate)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("CalculateLift() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChiSquaredPValue(t *testing.T) {
	tests := []struct {
		name       string
		chi2       float64
		df         int
		wantLow    bool // expect p < 0.05
	}{
		{
			name:    "high chi2 significant",
			chi2:    50,
			df:      5,
			wantLow: true,
		},
		{
			name:    "low chi2 not significant",
			chi2:    2,
			df:      5,
			wantLow: false,
		},
		{
			name:    "zero chi2",
			chi2:    0,
			df:      5,
			wantLow: false,
		},
		{
			name:    "zero df",
			chi2:    10,
			df:      0,
			wantLow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChiSquaredPValue(tt.chi2, tt.df)
			if tt.wantLow && got >= 0.05 {
				t.Errorf("ChiSquaredPValue() = %v, want < 0.05", got)
			}
			if !tt.wantLow && got < 0.05 {
				t.Errorf("ChiSquaredPValue() = %v, want >= 0.05", got)
			}
		})
	}
}

func TestDistributionToRates(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		total  int
		want   map[string]float64
	}{
		{
			name:   "normal conversion",
			counts: map[string]int{"a": 25, "b": 75},
			total:  100,
			want:   map[string]float64{"a": 0.25, "b": 0.75},
		},
		{
			name:   "zero total",
			counts: map[string]int{"a": 25},
			total:  0,
			want:   map[string]float64{},
		},
		{
			name:   "empty counts",
			counts: map[string]int{},
			total:  100,
			want:   map[string]float64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DistributionToRates(tt.counts, tt.total)
			if len(got) != len(tt.want) {
				t.Errorf("DistributionToRates() len = %v, want %v", len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if math.Abs(got[k]-v) > 0.001 {
					t.Errorf("DistributionToRates()[%s] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestFindTopValue(t *testing.T) {
	tests := []struct {
		name           string
		anomalous      map[string]int
		baseline       map[string]int
		anomalousTotal int
		baselineTotal  int
		wantValue      string
		wantLiftMin    float64
	}{
		{
			name:           "shard 7 example",
			anomalous:      map[string]int{"shard-7": 98, "shard-1": 1, "shard-2": 1},
			baseline:       map[string]int{"shard-7": 8, "shard-1": 23, "shard-2": 23, "shard-3": 23, "shard-4": 23},
			anomalousTotal: 100,
			baselineTotal:  100,
			wantValue:      "shard-7",
			wantLiftMin:    10.0,
		},
		{
			name:           "empty anomalous",
			anomalous:      map[string]int{},
			baseline:       map[string]int{"a": 50},
			anomalousTotal: 0,
			baselineTotal:  50,
			wantValue:      "",
			wantLiftMin:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotLift := FindTopValue(tt.anomalous, tt.baseline, tt.anomalousTotal, tt.baselineTotal)
			if gotValue != tt.wantValue {
				t.Errorf("FindTopValue() value = %v, want %v", gotValue, tt.wantValue)
			}
			if gotLift < tt.wantLiftMin {
				t.Errorf("FindTopValue() lift = %v, want >= %v", gotLift, tt.wantLiftMin)
			}
		})
	}
}
