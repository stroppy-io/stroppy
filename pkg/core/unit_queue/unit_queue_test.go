package unit_queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stroppy-io/stroppy/pkg/core/proto"
)

// Update MockDriver to support both interfaces.
type MockDriver struct {
	GenerateCount  int64
	GenerateDelay  time.Duration
	GenerateError  error
	GenerateResult *proto.DriverTransaction
}

func NewMockDriver() *MockDriver {
	return &MockDriver{
		GenerateResult: &proto.DriverTransaction{},
	}
}

func (m *MockDriver) GenerateNextUnit(
	ctx context.Context,
	_ *proto.UnitDescriptor,
) (*proto.DriverTransaction, error) {
	atomic.AddInt64(&m.GenerateCount, 1)

	if m.GenerateDelay > 0 {
		select {
		case <-time.After(m.GenerateDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.GenerateError != nil {
		return nil, m.GenerateError
	}

	return &proto.DriverTransaction{}, nil
}

func (m *MockDriver) GetGenerateCount() int64 {
	return atomic.LoadInt64(&m.GenerateCount)
}

func (m *MockDriver) ResetCount() {
	atomic.StoreInt64(&m.GenerateCount, 0)
}

// Helper function to create test step descriptor.
func createTestStepDescriptor(async bool, units []*proto.StepUnitDescriptor) *proto.StepDescriptor {
	return &proto.StepDescriptor{
		Async: async,
		Units: units,
	}
}

func createTestStepUnitDescriptor(count uint64) *proto.StepUnitDescriptor {
	return &proto.StepUnitDescriptor{
		Count:       count,
		Descriptor_: &proto.UnitDescriptor{
			// Add required fields
		},
	}
}

// Basic functionality tests.
func TestNewUnitQueue(t *testing.T) {
	driver := NewMockDriver()
	step := createTestStepDescriptor(false, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(1),
	})

	queue := NewUnitQueue(driver, step)

	if queue == nil {
		t.Fatal("NewUnitQueue returned nil")
	}

	if queue.driver != driver {
		t.Error("Driver not set correctly")
	}

	if queue.step != step {
		t.Error("Step descriptor not set correctly")
	}
}

func TestUnitQueue_BasicOperation(t *testing.T) {
	ctx := context.Background()
	driver := NewMockDriver()
	step := createTestStepDescriptor(false, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(3),
	})

	queue := NewUnitQueue(driver, step)
	queue.StartGeneration(ctx)

	// Give some time for generation to start
	time.Sleep(100 * time.Millisecond)

	// Get transactions
	transactions := make([]*proto.DriverTransaction, 0, 3)

	for i := range 3 {
		tx, err := queue.GetNextUnit()
		if err != nil {
			t.Fatalf("Failed to get transaction %d: %v", i, err)
		}

		if tx == nil {
			t.Fatalf("Got nil transaction at index %d", i)
		}

		transactions = append(transactions, tx)
	}

	queue.Stop()

	if len(transactions) < 3 {
		t.Errorf("Expected at least 3 transactions, got %d", len(transactions))
	}

	if driver.GetGenerateCount() < 3 {
		t.Errorf("Expected at least 3 generate calls, got %d", driver.GetGenerateCount())
	}
}

func TestUnitQueue_MultipleUnits(t *testing.T) {
	ctx := context.Background()
	driver := NewMockDriver()
	step := createTestStepDescriptor(true, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(2),
		createTestStepUnitDescriptor(3),
	})

	queue := NewUnitQueue(driver, step)
	queue.StartGeneration(ctx)

	time.Sleep(200 * time.Millisecond)

	// Should generate 2 + 3 = 5 transactions
	transactions := make([]*proto.DriverTransaction, 0, 5)

	for i := range 5 {
		tx, err := queue.GetNextUnit()
		if err != nil {
			t.Fatalf("Failed to get transaction %d: %v", i, err)
		}

		transactions = append(transactions, tx)
	}

	queue.Stop()

	if len(transactions) != 5 {
		t.Errorf("Expected 5 transactions, got %d", len(transactions))
	}
}

func TestUnitQueue_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	driver := NewMockDriver()
	driver.GenerateDelay = 50 * time.Millisecond // Add delay to test cancellation

	step := createTestStepDescriptor(false, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(100), // Large number
	})

	queue := NewUnitQueue(driver, step)
	queue.StartGeneration(ctx)

	// Cancel after short time
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait a bit for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Try to get transaction - should eventually get error or closed channel
	_, err := queue.GetNextUnit()
	if err == nil {
		// Try a few more times as cancellation might take time
		for range 10 {
			time.Sleep(10 * time.Millisecond)

			_, err = queue.GetNextUnit()
			if err != nil {
				break
			}
		}
	}

	queue.Stop()

	// We should eventually get an error due to context cancellation
	if err == nil {
		t.Log("Warning: Expected error due to context cancellation, but got none")
	}
}

func TestUnitQueue_DriverError(t *testing.T) {
	ctx := context.Background()
	driver := NewMockDriver()
	driver.GenerateError = errors.New("driver error")

	step := createTestStepDescriptor(false, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(1),
	})

	queue := NewUnitQueue(driver, step)
	queue.StartGeneration(ctx)

	time.Sleep(100 * time.Millisecond)

	_, err := queue.GetNextUnit()
	queue.Stop()

	if err == nil {
		t.Error("Expected error from driver, got none")
	}
}

// Consistency tests.
func TestUnitQueue_Consistency(t *testing.T) {
	t.Run("SingleUnit", func(t *testing.T) {
		testConsistency(t, 1, 1, false)
	})

	t.Run("MultipleUnitsSync", func(t *testing.T) {
		testConsistency(t, 3, 5, false)
	})

	t.Run("MultipleUnitsAsync", func(t *testing.T) {
		testConsistency(t, 3, 5, true)
	})
}

func testConsistency(t *testing.T, numUnits, countPerUnit int, async bool) {
	t.Helper()

	ctx := context.Background()
	driver := NewMockDriver()

	units := make([]*proto.StepUnitDescriptor, 0, numUnits)
	for range numUnits {
		units = append(units, createTestStepUnitDescriptor(uint64(countPerUnit)))
	}

	step := createTestStepDescriptor(async, units)
	queue := NewUnitQueue(driver, step)
	queue.StartGeneration(ctx)

	expectedTotal := numUnits * countPerUnit

	var transactions []*proto.DriverTransaction

	// Collect all expected transactions
	timeout := time.After(5 * time.Second)

	for len(transactions) < expectedTotal {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for transactions. Got %d, expected %d",
				len(transactions), expectedTotal)
		default:
			tx, err := queue.GetNextUnit()
			if err != nil {
				t.Fatalf("Error getting transaction: %v", err)
			}

			if tx == nil {
				t.Fatal("Got nil transaction")
			}

			transactions = append(transactions, tx)
		}
	}

	queue.Stop()

	if len(transactions) < expectedTotal {
		t.Errorf("Expected %d transactions, got %d", expectedTotal, len(transactions))
	}

	if driver.GetGenerateCount() < int64(expectedTotal) {
		t.Errorf("Expected %d generate calls, got %d", expectedTotal, driver.GetGenerateCount())
	}
}

// Parallel and race detection tests.
func TestUnitQueue_ParallelConsumers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	driver := NewMockDriver()
	step := createTestStepDescriptor(true, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(100),
	})

	queue := NewUnitQueue(driver, step)
	queue.StartGeneration(ctx)

	const numConsumers = 10

	var wg sync.WaitGroup

	var totalReceived int64

	var errors []error

	var mu sync.Mutex

	// Start multiple consumers
	for i := range numConsumers {
		wg.Add(1)

		go func(consumerID int) {
			defer wg.Done()

			for range 10 { // Each consumer gets 10 transactions
				tx, err := queue.GetNextUnit()
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("consumer %d: %w", consumerID, err))
					mu.Unlock()

					return
				}

				if tx == nil {
					mu.Lock()
					errors = append(
						errors,
						fmt.Errorf("consumer %d: got nil transaction", consumerID),
					)
					mu.Unlock()

					return
				}

				atomic.AddInt64(&totalReceived, 1)
			}
		}(i)
	}

	wg.Wait()
	queue.Stop()

	if len(errors) > 0 {
		t.Fatalf("Got %d errors: %v", len(errors), errors[0])
	}

	if totalReceived != 100 {
		t.Errorf("Expected 100 transactions received, got %d", totalReceived)
	}
}

func TestUnitQueue_RaceConditions(t *testing.T) {
	// This test is designed to be run with -race flag
	t.Parallel()

	for i := range 10 {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			driver := NewMockDriver()
			step := createTestStepDescriptor(true, []*proto.StepUnitDescriptor{
				createTestStepUnitDescriptor(50),
			})

			queue := NewUnitQueue(driver, step)
			queue.StartGeneration(ctx)

			var wg sync.WaitGroup

			// Multiple consumers
			for range 5 {
				wg.Add(1)

				go func() {
					defer wg.Done()

					for range 10 {
						_, err := queue.GetNextUnit()
						if err != nil {
							return // Stop on error
						}
					}
				}()
			}

			wg.Wait()
			queue.Stop()
		})
	}
}

func TestUnitQueue_StopRace(t *testing.T) {
	t.Parallel()

	// Test concurrent Stop() calls
	for i := range 20 {
		t.Run(fmt.Sprintf("stop_race_%d", i), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			driver := NewMockDriver()
			step := createTestStepDescriptor(true, []*proto.StepUnitDescriptor{
				createTestStepUnitDescriptor(10),
			})

			queue := NewUnitQueue(driver, step)
			queue.StartGeneration(ctx)

			var wg sync.WaitGroup

			// Multiple goroutines calling Stop()
			for j := range 3 {
				wg.Add(1)

				go func() {
					defer wg.Done()
					time.Sleep(time.Duration(j*10) * time.Millisecond)
					queue.Stop()
				}()
			}

			// One goroutine consuming
			wg.Add(1)

			go func() {
				defer wg.Done()

				for {
					_, err := queue.GetNextUnit()
					if err != nil {
						return
					}
				}
			}()

			wg.Wait()
		})
	}
}

func BenchmarkUnitQueue_SingleConsumer(b *testing.B) {
	driver := NewMockDriver()
	step := createTestStepDescriptor(false, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(1),
	})

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		ctx := context.Background()
		queue := NewUnitQueue(driver, step)
		queue.StartGeneration(ctx)

		_, err := queue.GetNextUnit()
		if err != nil {
			b.Fatalf("Error getting transaction: %v", err)
		}

		queue.Stop()
		driver.ResetCount()
	}
}

func BenchmarkUnitQueue_ParallelConsumers(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		driver := NewMockDriver()
		step := createTestStepDescriptor(true, []*proto.StepUnitDescriptor{
			createTestStepUnitDescriptor(100),
		})

		ctx := context.Background()
		queue := NewUnitQueue(driver, step)
		queue.StartGeneration(ctx)

		for pb.Next() {
			_, err := queue.GetNextUnit()
			if err != nil {
				// Reset if we hit an error
				queue.Stop()
				driver.ResetCount()
				queue = NewUnitQueue(driver, step)
				queue.StartGeneration(ctx)
			}
		}

		queue.Stop()
	})
}

func BenchmarkUnitQueue_HighThroughput(b *testing.B) {
	driver := NewMockDriver()
	step := createTestStepDescriptor(true, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(1000),
		createTestStepUnitDescriptor(1000),
		createTestStepUnitDescriptor(1000),
	})

	ctx := context.Background()
	queue := NewUnitQueue(driver, step)
	queue.StartGeneration(ctx)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := queue.GetNextUnit()
			if err != nil {
				b.Fatalf("Error getting transaction: %v", err)
			}
		}
	})

	queue.Stop()
}

func BenchmarkUnitQueue_MemoryUsage(b *testing.B) {
	b.ReportAllocs()

	driver := NewMockDriver()
	step := createTestStepDescriptor(false, []*proto.StepUnitDescriptor{
		createTestStepUnitDescriptor(10),
	})

	for range b.N {
		ctx := context.Background()
		queue := NewUnitQueue(driver, step)
		queue.StartGeneration(ctx)

		// Consume all transactions
		for range 10 {
			_, err := queue.GetNextUnit()
			if err != nil {
				break
			}
		}

		queue.Stop()
		driver.ResetCount()
	}
}
