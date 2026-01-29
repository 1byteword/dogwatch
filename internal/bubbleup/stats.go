package bubbleup

import "math"

// ChiSquared computes the chi-squared statistic for two distributions
// observed and expected should be aligned arrays of counts
func ChiSquared(observed, expected []float64) float64 {
	if len(observed) != len(expected) {
		return 0
	}

	var chi2 float64
	for i := range observed {
		if expected[i] > 0 {
			diff := observed[i] - expected[i]
			chi2 += (diff * diff) / expected[i]
		}
	}
	return chi2
}

// ChiSquaredFromDistributions computes chi-squared from two count maps
func ChiSquaredFromDistributions(anomalous, baseline map[string]int, anomalousTotal, baselineTotal int) float64 {
	if anomalousTotal == 0 || baselineTotal == 0 {
		return 0
	}

	// Collect all unique values
	allValues := make(map[string]bool)
	for k := range anomalous {
		allValues[k] = true
	}
	for k := range baseline {
		allValues[k] = true
	}

	var chi2 float64
	for value := range allValues {
		anomalousCount := float64(anomalous[value])
		baselineCount := float64(baseline[value])

		// Expected count if distributions were the same
		totalCount := anomalousCount + baselineCount
		totalN := float64(anomalousTotal + baselineTotal)

		expectedAnomalous := totalCount * float64(anomalousTotal) / totalN
		expectedBaseline := totalCount * float64(baselineTotal) / totalN

		if expectedAnomalous > 0 {
			diff := anomalousCount - expectedAnomalous
			chi2 += (diff * diff) / expectedAnomalous
		}
		if expectedBaseline > 0 {
			diff := baselineCount - expectedBaseline
			chi2 += (diff * diff) / expectedBaseline
		}
	}

	return chi2
}

// CalculateLift computes how much more likely a value is in the anomalous set
func CalculateLift(anomalousRate, baselineRate float64) float64 {
	if baselineRate <= 0 {
		if anomalousRate > 0 {
			return 100.0 // Infinite lift capped at 100x
		}
		return 1.0
	}
	lift := anomalousRate / baselineRate
	if lift > 100 {
		return 100.0
	}
	return lift
}

// ChiSquaredPValue approximates the p-value for a chi-squared statistic
// Uses Wilson-Hilferty approximation for large degrees of freedom
func ChiSquaredPValue(chi2 float64, df int) float64 {
	if df <= 0 || chi2 <= 0 {
		return 1.0
	}

	// Wilson-Hilferty transformation
	dfFloat := float64(df)
	z := math.Pow(chi2/dfFloat, 1.0/3.0) - (1.0 - 2.0/(9.0*dfFloat))
	z = z / math.Sqrt(2.0/(9.0*dfFloat))

	// Standard normal CDF approximation
	return 1.0 - normalCDF(z)
}

// normalCDF approximates the standard normal cumulative distribution function
func normalCDF(z float64) float64 {
	// Abramowitz and Stegun approximation
	const (
		a1 = 0.254829592
		a2 = -0.284496736
		a3 = 1.421413741
		a4 = -1.453152027
		a5 = 1.061405429
		p  = 0.3275911
	)

	sign := 1.0
	if z < 0 {
		sign = -1.0
		z = -z
	}

	t := 1.0 / (1.0 + p*z)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-z*z/2.0)

	return 0.5 * (1.0 + sign*y)
}

// DistributionToRates converts count map to rate map
func DistributionToRates(counts map[string]int, total int) map[string]float64 {
	rates := make(map[string]float64)
	if total == 0 {
		return rates
	}
	for k, v := range counts {
		rates[k] = float64(v) / float64(total)
	}
	return rates
}

// FindTopValue finds the value with highest lift between anomalous and baseline
func FindTopValue(anomalous, baseline map[string]int, anomalousTotal, baselineTotal int) (string, float64) {
	var topValue string
	var topLift float64

	for value, count := range anomalous {
		if count == 0 {
			continue
		}
		anomalousRate := float64(count) / float64(anomalousTotal)
		baselineRate := float64(baseline[value]) / float64(baselineTotal)
		lift := CalculateLift(anomalousRate, baselineRate)

		if lift > topLift {
			topLift = lift
			topValue = value
		}
	}

	return topValue, topLift
}
