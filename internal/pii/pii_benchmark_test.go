package pii

import (
	"strings"
	"sync"
	"testing"
)

// Test data for benchmarks
var (
	testEmail       = "Contact us at john.doe@example.com for more info"
	testCreditCard  = "Payment with card 4532015112830366 processed"
	testSSN         = "SSN: 123-45-6789 on file"
	testPhone       = "Call us at +1-555-123-4567 today"
	testAWSKey      = "AWS key: AKIAIOSFODNN7EXAMPLE found in config"
	testMixedPII    = "User john.doe@example.com with SSN 123-45-6789 called +1-555-123-4567 and paid with 4532015112830366"
	testNoPII       = "This is a completely normal log message without any sensitive data."
	testJWT         = "Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	testGitHubToken = "Github: ghp_1234567890abcdefghijABCDEFGHIJab12"
)

// Generate large text for stress testing
func generateLargeText(size int) string {
	// Mix of normal text and PII scattered throughout
	templates := []string{
		"This is a normal log line without any sensitive information. ",
		"User logged in from IP 192.168.1.100 at timestamp 2024-01-15T10:30:00Z. ",
		"Email sent to user@example.com successfully. ",
		"Processing payment for card 4532015112830366. ",
		"Employee SSN 123-45-6789 verified. ",
		"Contact support at +1-555-987-6543 for help. ",
		"AWS access key AKIAIOSFODNN7EXAMPLE detected. ",
		"Debug: Connection established to database server on port 5432. ",
	}

	var sb strings.Builder
	templateIdx := 0
	for sb.Len() < size {
		sb.WriteString(templates[templateIdx%len(templates)])
		templateIdx++
	}
	return sb.String()
}

func BenchmarkDetectEmail(b *testing.B) {
	detector := NewDetector(nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(testEmail)
	}
}

func BenchmarkDetectCreditCard(b *testing.B) {
	detector := NewDetector(nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(testCreditCard)
	}
}

func BenchmarkDetectSSN(b *testing.B) {
	detector := NewDetector(nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(testSSN)
	}
}

func BenchmarkDetectPhone(b *testing.B) {
	detector := NewDetector(nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(testPhone)
	}
}

func BenchmarkDetectAll(b *testing.B) {
	detector := NewDetector(nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(testMixedPII)
	}
}

func BenchmarkDetectNoPII(b *testing.B) {
	detector := NewDetector(nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(testNoPII)
	}
}

func BenchmarkDetectJWT(b *testing.B) {
	detector := NewDetector(nil)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(testJWT)
	}
}

func BenchmarkRedactMask(b *testing.B) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyMask
	redactor := NewRedactor(config)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = redactor.Redact(testMixedPII)
	}
}

func BenchmarkRedactHash(b *testing.B) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyHash
	for k := range config.TypeConfigs {
		tc := config.TypeConfigs[k]
		tc.Strategy = StrategyHash
		config.TypeConfigs[k] = tc
	}
	redactor := NewRedactor(config)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = redactor.Redact(testMixedPII)
	}
}

func BenchmarkRedactTokenize(b *testing.B) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyTokenize
	for k := range config.TypeConfigs {
		tc := config.TypeConfigs[k]
		tc.Strategy = StrategyTokenize
		config.TypeConfigs[k] = tc
	}
	redactor := NewRedactor(config)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = redactor.Redact(testMixedPII)
	}
}

func BenchmarkRedactFormat(b *testing.B) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyFormat
	for k := range config.TypeConfigs {
		tc := config.TypeConfigs[k]
		tc.Strategy = StrategyFormat
		config.TypeConfigs[k] = tc
	}
	redactor := NewRedactor(config)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = redactor.Redact(testMixedPII)
	}
}

func BenchmarkScanLargeText(b *testing.B) {
	// Generate 1MB text
	largeText := generateLargeText(1024 * 1024)
	detector := NewDetector(nil)

	b.ReportAllocs()
	b.SetBytes(int64(len(largeText)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(largeText)
	}
}

// Table-driven benchmark for different text sizes
func BenchmarkDetectBySize(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	detector := NewDetector(nil)

	for _, size := range sizes {
		text := generateLargeText(size.size)
		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(text)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = detector.Detect(text)
			}
		})
	}
}

func BenchmarkConcurrentDetection(b *testing.B) {
	detector := NewDetector(nil)
	texts := []string{testEmail, testCreditCard, testSSN, testPhone, testMixedPII}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = detector.Detect(texts[i%len(texts)])
			i++
		}
	})
}

func BenchmarkConcurrentRedaction(b *testing.B) {
	redactor := NewRedactor(nil)
	texts := []string{testEmail, testCreditCard, testSSN, testPhone, testMixedPII}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = redactor.Redact(texts[i%len(texts)])
			i++
		}
	})
}

func BenchmarkHasPII(b *testing.B) {
	detector := NewDetector(nil)

	b.Run("WithPII", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = detector.HasPII(testMixedPII)
		}
	})

	b.Run("WithoutPII", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = detector.HasPII(testNoPII)
		}
	})
}

func BenchmarkRedactAttributes(b *testing.B) {
	redactor := NewRedactor(nil)
	attrs := map[string]string{
		"user.email":     "john.doe@example.com",
		"user.phone":     "+1-555-123-4567",
		"db.statement":   "SELECT * FROM users WHERE ssn = '123-45-6789'",
		"http.url":       "https://api.example.com/users?email=test@test.com",
		"message":        "Payment processed for card 4532015112830366",
		"error.message":  "Auth failed for user admin@example.com",
		"trace_id":       "abc123def456",
		"request.method": "POST",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = redactor.RedactAttributes(attrs)
	}
}

func BenchmarkLuhnValidation(b *testing.B) {
	validCards := []string{
		"4532015112830366",
		"5425233430109903",
		"374245455400126",
		"6011514433546201",
	}
	invalidCards := []string{
		"1234567890123456",
		"0000000000000000",
		"9999999999999999",
	}

	b.Run("ValidCards", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, card := range validCards {
				_ = luhnCheck(card)
			}
		}
	})

	b.Run("InvalidCards", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, card := range invalidCards {
				_ = luhnCheck(card)
			}
		}
	})
}

// Benchmark with custom patterns enabled
func BenchmarkDetectWithCustomPatterns(b *testing.B) {
	config := DefaultConfig()
	config.AddCustomPattern("internal_id", `\bINT-[0-9]{8}\b`, StrategyMask)
	config.AddCustomPattern("order_ref", `\bORD-[A-Z0-9]{10}\b`, StrategyMask)
	config.AddCustomPattern("employee_id", `\bEMP[0-9]{6}\b`, StrategyMask)

	detector := NewDetector(config)
	text := "Order ORD-ABC1234567 for employee EMP123456 with internal ref INT-12345678"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.Detect(text)
	}
}

// Benchmark allowlist checking
func BenchmarkAllowlistCheck(b *testing.B) {
	config := DefaultConfig()
	// Add 100 items to allowlist
	for i := 0; i < 100; i++ {
		config.AddToAllowlist(strings.Repeat("x", 20))
	}

	b.Run("ValueNotInList", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = config.IsAllowed("unknown-value")
		}
	})

	config.AddToAllowlist("test@example.com")
	b.Run("ValueInList", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = config.IsAllowed("test@example.com")
		}
	})
}

// Benchmark concurrent access to config
func BenchmarkConfigConcurrentAccess(b *testing.B) {
	config := DefaultConfig()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var wg sync.WaitGroup
		for pb.Next() {
			wg.Add(2)
			go func() {
				defer wg.Done()
				_ = config.IsTypeEnabled(TypeEmail)
			}()
			go func() {
				defer wg.Done()
				_ = config.GetStrategy(TypeCreditCard)
			}()
			wg.Wait()
		}
	})
}
