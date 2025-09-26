package unit_queue_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/pkg/core/unit_queue"
)

func NewQueueExample() {
	generator := func(_ context.Context, x int) (int, error) { return x * 42, nil }
	// setup a generator function, workers limit and internal buffer size
	queue := unit_queue.NewQueue(generator, 1, 100)

	for _, seed := range []int{1, 2, 3, 4} {
		// put the initial value, generator will use
		// also how many workers will run it simultaniously
		// and how many times generator will be started with this value
		queue.PrepareGenerator(seed, 1, 1)
	}

	// start the value generation
	queue.StartGeneration(context.Background())

	for range 5 {
		// take value by one in any concurent context
		value, _ := queue.GetNextElement()
		fmt.Println(value)
	}

	queue.Stop()

	// Output: 42
	// 84
	// 126
	// 168
	// 210
}

// Test basic functionality
func TestQueuedGenerator_BasicOperation(t *testing.T) {
	generator := func(_ context.Context, x int) (int, error) {
		return x * 10, nil
	}

	queue := unit_queue.NewQueue(generator, 2, 100)

	// Add seeds
	queue.PrepareGenerator(1, 1, 2) // should produce 10, 10
	queue.PrepareGenerator(2, 1, 1) // should produce 20

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	// Collect results
	results := make(map[int]int)
	expectedCount := 3 // 2 + 1 = 3 total results per cycle

	for i := 0; i < expectedCount*2; i++ { // Get 2 cycles worth
		value, err := queue.GetNextElement()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		results[value]++
	}

	queue.Stop()

	// Verify proportions (values should appear according to their repeat counts)
	if results[10] == 0 || results[20] == 0 {
		t.Errorf("Missing expected values. Got: %v", results)
	}

	// In 2 cycles: 10 should appear 4 times (2*2), 20 should appear 2 times (1*2)
	expectedRatio := float64(results[10]) / float64(results[20])
	if math.Abs(expectedRatio-2.0) > 0.1 {
		t.Errorf("Expected ratio ~2:1, got %f", expectedRatio)
	}
}

// Test the proportional distribution property
func TestQueuedGenerator_ProportionalDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proportional distribution test in short mode")
	}

	generator := func(_ context.Context, seed int) (int, error) {
		return seed, nil
	}

	queue := unit_queue.NewQueue(generator, 10, 1000)

	// Configure seeds with different repeat counts
	// Seed 1: 1*50 = 50 per cycle
	// Seed 2: 1*20 = 20 per cycle
	// Seed 3: 1*30 = 30 per cycle
	// Total per cycle: 100, ratios 5:2:3
	queue.PrepareGenerator(1, 1, 50)
	queue.PrepareGenerator(2, 1, 20)
	queue.PrepareGenerator(3, 1, 30)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	// Collect large number of samples
	const sampleSize = 100000
	counts := make(map[int]int64)

	for i := 0; i < sampleSize; i++ {
		value, err := queue.GetNextElement()
		if err != nil {
			if errors.Is(err, unit_queue.ErrQueueIsDead) {
				break
			}
			t.Fatalf("Unexpected error at sample %d: %v", i, err)
		}
		var some = counts[value]
		atomic.AddInt64(&some, 1)
	}

	queue.Stop()

	total := counts[1] + counts[2] + counts[3]
	if total == 0 {
		t.Fatal("No values collected")
	}

	// Calculate actual ratios
	ratio1 := float64(counts[1]) / float64(total)
	ratio2 := float64(counts[2]) / float64(total)
	ratio3 := float64(counts[3]) / float64(total)

	// Expected ratios: 5:2:3 = 50%, 20%, 30%
	expectedRatio1 := 0.50
	expectedRatio2 := 0.20
	expectedRatio3 := 0.30

	tolerance := 0.05 // 5% tolerance

	t.Logf("Collected %d samples. Ratios: %.3f:%.3f:%.3f (expected: %.3f:%.3f:%.3f)",
		total, ratio1, ratio2, ratio3, expectedRatio1, expectedRatio2, expectedRatio3)

	if math.Abs(ratio1-expectedRatio1) > tolerance {
		t.Errorf("Seed 1 ratio %.3f differs from expected %.3f by more than %.3f",
			ratio1, expectedRatio1, tolerance)
	}
	if math.Abs(ratio2-expectedRatio2) > tolerance {
		t.Errorf("Seed 2 ratio %.3f differs from expected %.3f by more than %.3f",
			ratio2, expectedRatio2, tolerance)
	}
	if math.Abs(ratio3-expectedRatio3) > tolerance {
		t.Errorf("Seed 3 ratio %.3f differs from expected %.3f by more than %.3f",
			ratio3, expectedRatio3, tolerance)
	}
}

// Test concurrent access safety
func TestQueuedGenerator_ConcurrentAccess(t *testing.T) {
	generator := func(_ context.Context, x int) (int, error) {
		time.Sleep(time.Microsecond) // Simulate work
		return x, nil
	}

	queue := unit_queue.NewQueue(generator, 5, 100)
	queue.PrepareGenerator(42, 3, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	// Multiple concurrent consumers
	const numConsumers = 10
	const samplesPerConsumer = 50

	var wg sync.WaitGroup
	results := make([][]int, numConsumers)
	errors := make([]error, numConsumers)

	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func(consumerID int) {
			defer wg.Done()

			var samples []int
			for j := 0; j < samplesPerConsumer; j++ {
				value, err := queue.GetNextElement()
				if err != nil {
					errors[consumerID] = err
					return
				}
				samples = append(samples, value)
			}
			results[consumerID] = samples
		}(i)
	}

	wg.Wait()
	queue.Stop()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("Consumer %d encountered error: %v", i, err)
		}
	}

	// Verify all consumers got expected values
	totalSamples := 0
	for i, samples := range results {
		if len(samples) != samplesPerConsumer {
			t.Errorf("Consumer %d got %d samples, expected %d",
				i, len(samples), samplesPerConsumer)
		}
		for _, value := range samples {
			if value != 42 {
				t.Errorf("Consumer %d got unexpected value %d", i, value)
			}
		}
		totalSamples += len(samples)
	}

	expectedTotal := numConsumers * samplesPerConsumer
	if totalSamples != expectedTotal {
		t.Errorf("Total samples %d, expected %d", totalSamples, expectedTotal)
	}
}

// Test race conditions
func TestQueuedGenerator_RaceConditions(t *testing.T) {
	// Test race between Stop() and GetNextElement()
	t.Run("stop_get_race", func(t *testing.T) {
		generator := func(ctx context.Context, x int) (int, error) {
			time.Sleep(time.Millisecond)
			return x, nil
		}

		queue := unit_queue.NewQueue(generator, 2, 10)
		queue.PrepareGenerator(1, 1, 100)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		queue.StartGeneration(ctx)

		var wg sync.WaitGroup
		errChan := make(chan error, 2)

		// Consumer goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				_, err := queue.GetNextElement()
				if err != nil {
					if !errors.Is(err, unit_queue.ErrQueueIsDead) &&
						!errors.Is(err, unit_queue.ErrQueueIsStopped) {
						errChan <- err
					}
					return
				}
			}
		}()

		// Stopper goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(5 * time.Millisecond)
			if err := queue.Stop(); err != nil {
				errChan <- err
			}
		}()

		wg.Wait()
		close(errChan)

		for err := range errChan {
			t.Errorf("Race condition error: %v", err)
		}
	})
}

// Test error propagation
func TestQueuedGenerator_ErrorHandling(t *testing.T) {
	testError := errors.New("generator error")
	callCount := int64(0)

	generator := func(_ context.Context, x int) (int, error) {
		count := atomic.AddInt64(&callCount, 1)
		if count == 5 { // Fail on 5th call
			return 0, testError
		}
		return x, nil
	}

	queue := unit_queue.NewQueue(generator, 2, 10)
	queue.PrepareGenerator(1, 2, 10) // Should trigger error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	// Consume until we hit the error
	var lastErr error
	for i := 0; i < 20; i++ {
		_, err := queue.GetNextElement()
		if err != nil {
			lastErr = err
			break
		}
	}

	queue.Stop()

	if lastErr == nil {
		t.Error("Expected error but got none")
	}
}

// Test goroutine leak detection
func TestQueuedGenerator_GoroutineLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping goroutine leak test in short mode")
	}

	initialGoroutines := runtime.NumGoroutine()

	generator := func(ctx context.Context, x int) (int, error) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Millisecond):
			return x, nil
		}
	}

	// Run multiple short-lived queues
	for i := 0; i < 50; i++ {
		func() {
			queue := unit_queue.NewQueue(generator, 2, 10)
			queue.PrepareGenerator(i, 1, 5)

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			queue.StartGeneration(ctx)

			// Consume a few values
			for j := 0; j < 3; j++ {
				_, err := queue.GetNextElement()
				if err != nil {
					break
				}
			}

			queue.Stop()
		}()
	}

	// Allow cleanup time
	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	runtime.GC() // Force GC
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	t.Logf("Goroutines: initial=%d, final=%d, leaked=%d",
		initialGoroutines, finalGoroutines, leaked)

	// Allow some tolerance for test framework goroutines
	if leaked > 5 {
		t.Errorf("Possible goroutine leak: %d goroutines remain", leaked)
	}
}

// Test worker count limits
func TestQueuedGenerator_WorkerLimits(t *testing.T) {
	activeWorkers := int64(0)
	maxActiveWorkers := int64(0)

	generator := func(_ context.Context, x int) (int, error) {
		current := atomic.AddInt64(&activeWorkers, 1)
		defer atomic.AddInt64(&activeWorkers, -1)

		// Track maximum concurrent workers
		for {
			max := atomic.LoadInt64(&maxActiveWorkers)
			if current <= max || atomic.CompareAndSwapInt64(&maxActiveWorkers, max, current) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond) // Hold worker for some time
		return x, nil
	}

	const workerLimit = 3
	queue := unit_queue.NewQueue(generator, workerLimit, 100)
	queue.PrepareGenerator(1, 10, 5) // Try to create 10 workers, but limit is 3

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	// Consume some values to trigger workers
	for i := 0; i < 20; i++ {
		_, err := queue.GetNextElement()
		if err != nil {
			break
		}
	}

	queue.Stop()

	maxWorkers := atomic.LoadInt64(&maxActiveWorkers)
	t.Logf("Maximum concurrent workers observed: %d (limit: %d)", maxWorkers, workerLimit)

	if maxWorkers > int64(workerLimit) {
		t.Errorf("Worker limit violated: observed %d workers, limit %d",
			maxWorkers, workerLimit)
	}
}

// BENCHMARKS

func BenchmarkQueuedGenerator_SingleWorker(b *testing.B) {
	generator := func(_ context.Context, x int) (int, error) {
		return x * 2, nil
	}

	queue := unit_queue.NewQueue(generator, 1, 1000)
	queue.PrepareGenerator(1, 1, 1000)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := queue.GetNextElement()
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	queue.Stop()
}

func BenchmarkQueuedGenerator_MultipleWorkers(b *testing.B) {
	generator := func(_ context.Context, x int) (int, error) {
		return x * 2, nil
	}

	queue := unit_queue.NewQueue(generator, 10, 10000)
	queue.PrepareGenerator(1, 5, 1000)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := queue.GetNextElement()
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	queue.Stop()
}

func BenchmarkQueuedGenerator_WorkerScaling(b *testing.B) {
	generator := func(_ context.Context, x int) (int, error) {
		// Simulate some work
		sum := 0
		for i := 0; i < 100; i++ {
			sum += i * x
		}
		return sum, nil
	}

	workerCounts := []int{1, 2, 4, 8, 16}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			queue := unit_queue.NewQueue(generator, workers, 1000)
			queue.PrepareGenerator(1, uint(workers), 1000)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			queue.StartGeneration(ctx)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := queue.GetNextElement()
				if err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()
			queue.Stop()
		})
	}
}

// Test for exact proportions with large sample size
func TestQueuedGenerator_ExactProportions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping exact proportions test in short mode")
	}

	generator := func(_ context.Context, seed int) (int, error) {
		return seed, nil
	}

	// Test the exact scenario from the user query
	queue := unit_queue.NewQueue(generator, 3, 10000) // Large capacity

	// Generator 1: 1*50 = 50 per cycle
	// Generator 2: 1*20 = 20 per cycle
	// Generator 3: 1*30 = 30 per cycle
	// Ratio: 5:2:3
	queue.PrepareGenerator(1, 1, 50)
	queue.PrepareGenerator(2, 1, 20)
	queue.PrepareGenerator(3, 1, 30)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	queue.StartGeneration(ctx)

	// Collect 100,000 samples as requested
	const targetSamples = 100000
	counts := make(map[int]int64)

	for i := 0; i < targetSamples; i++ {
		if i%10000 == 0 {
			t.Logf("Processed %d samples...", i)
		}

		value, err := queue.GetNextElement()
		if err != nil {
			t.Fatalf("Error at sample %d: %v", i, err)
		}

		if value < 1 || value > 3 {
			t.Fatalf("Unexpected value %d at sample %d", value, i)
		}

		some := counts[value]
		atomic.AddInt64(&some, 1)
	}

	queue.Stop()

	total := counts[1] + counts[2] + counts[3]
	t.Logf("Final counts: %d:%d:%d (total: %d)", counts[1], counts[2], counts[3], total)

	// Calculate exact ratios
	ratio1 := float64(counts[1]) / float64(total)
	ratio2 := float64(counts[2]) / float64(total)
	ratio3 := float64(counts[3]) / float64(total)

	// Expected: 50%, 20%, 30%
	expected1, expected2, expected3 := 0.5, 0.2, 0.3

	// Very strict tolerance for large sample
	tolerance := 0.01 // 1%

	t.Logf("Actual ratios: %.4f:%.4f:%.4f", ratio1, ratio2, ratio3)
	t.Logf("Expected ratios: %.4f:%.4f:%.4f", expected1, expected2, expected3)

	if math.Abs(ratio1-expected1) > tolerance {
		t.Errorf("Generator 1 ratio %.4f differs from expected %.4f by %.4f (tolerance: %.4f)",
			ratio1, expected1, math.Abs(ratio1-expected1), tolerance)
	}
	if math.Abs(ratio2-expected2) > tolerance {
		t.Errorf("Generator 2 ratio %.4f differs from expected %.4f by %.4f (tolerance: %.4f)",
			ratio2, expected2, math.Abs(ratio2-expected2), tolerance)
	}
	if math.Abs(ratio3-expected3) > tolerance {
		t.Errorf("Generator 3 ratio %.4f differs from expected %.4f by %.4f (tolerance: %.4f)",
			ratio3, expected3, math.Abs(ratio3-expected3), tolerance)
	}

	// Additional check: verify the exact ratio relationships
	// Ratio should be 50:20:30 = 5:2:3
	ratio12 := float64(counts[1]) / float64(counts[2]) // Should be ~2.5 (5/2)
	ratio13 := float64(counts[1]) / float64(counts[3]) // Should be ~1.67 (5/3)
	ratio23 := float64(counts[2]) / float64(counts[3]) // Should be ~0.67 (2/3)

	expected12, expected13, expected23 := 2.5, 5.0/3.0, 2.0/3.0

	t.Logf("Ratio relationships: 1:2=%.3f (exp=%.3f), 1:3=%.3f (exp=%.3f), 2:3=%.3f (exp=%.3f)",
		ratio12, expected12, ratio13, expected13, ratio23, expected23)

	ratioTolerance := 0.05 // 5% for ratio relationships

	if math.Abs(ratio12-expected12) > ratioTolerance {
		t.Errorf("Ratio 1:2 = %.3f differs from expected %.3f", ratio12, expected12)
	}
	if math.Abs(ratio13-expected13) > ratioTolerance {
		t.Errorf("Ratio 1:3 = %.3f differs from expected %.3f", ratio13, expected13)
	}
	if math.Abs(ratio23-expected23) > ratioTolerance {
		t.Errorf("Ratio 2:3 = %.3f differs from expected %.3f", ratio23, expected23)
	}
}
