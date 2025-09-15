package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/NikitaDmitryuk/telegram-media-server/internal/core/domain"
	"github.com/NikitaDmitryuk/telegram-media-server/internal/pkg/logger"
)

const (
	// MaxHistogramValues максимальное количество значений в гистограмме
	MaxHistogramValues = 1000
)

// InMemoryMetrics реализует простую in-memory систему метрик
type InMemoryMetrics struct {
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
	durations  map[string]*Duration
	mu         sync.RWMutex
}

// Counter представляет счетчик
type Counter struct {
	Value  int64             `json:"value"`
	Labels map[string]string `json:"labels"`
	mu     sync.RWMutex
}

// Gauge представляет gauge метрику
type Gauge struct {
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels"`
	mu     sync.RWMutex
}

// Histogram представляет гистограмму
type Histogram struct {
	Values []float64         `json:"values"`
	Labels map[string]string `json:"labels"`
	mu     sync.RWMutex
}

// Duration представляет метрику времени выполнения
type Duration struct {
	Values []time.Duration   `json:"values"`
	Labels map[string]string `json:"labels"`
	mu     sync.RWMutex
}

// NewInMemoryMetrics создает новую in-memory систему метрик
func NewInMemoryMetrics() domain.MetricsInterface {
	return &InMemoryMetrics{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
		durations:  make(map[string]*Duration),
	}
}

// IncrementCounter увеличивает счетчик
func (m *InMemoryMetrics) IncrementCounter(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	counter, exists := m.counters[key]
	if !exists {
		counter = &Counter{
			Value:  0,
			Labels: copyLabels(labels),
		}
		m.counters[key] = counter
	}

	counter.mu.Lock()
	counter.Value++
	counter.mu.Unlock()

	logger.Log.WithFields(map[string]any{
		"metric": name,
		"labels": labels,
		"value":  counter.Value,
	}).Debug("Counter incremented")
}

// SetGauge устанавливает значение gauge
func (m *InMemoryMetrics) SetGauge(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	gauge, exists := m.gauges[key]
	if !exists {
		gauge = &Gauge{
			Labels: copyLabels(labels),
		}
		m.gauges[key] = gauge
	}

	gauge.mu.Lock()
	gauge.Value = value
	gauge.mu.Unlock()

	logger.Log.WithFields(map[string]any{
		"metric": name,
		"labels": labels,
		"value":  value,
	}).Debug("Gauge set")
}

// RecordHistogram записывает значение в гистограмму
func (m *InMemoryMetrics) RecordHistogram(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	histogram, exists := m.histograms[key]
	if !exists {
		histogram = &Histogram{
			Values: make([]float64, 0),
			Labels: copyLabels(labels),
		}
		m.histograms[key] = histogram
	}

	histogram.mu.Lock()
	histogram.Values = append(histogram.Values, value)
	// Ограничиваем размер истории
	if len(histogram.Values) > MaxHistogramValues {
		histogram.Values = histogram.Values[len(histogram.Values)-1000:]
	}
	histogram.mu.Unlock()

	logger.Log.WithFields(map[string]any{
		"metric": name,
		"labels": labels,
		"value":  value,
	}).Debug("Histogram recorded")
}

// RecordDuration записывает время выполнения
func (m *InMemoryMetrics) RecordDuration(name string, duration time.Duration, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	durationMetric, exists := m.durations[key]
	if !exists {
		durationMetric = &Duration{
			Values: make([]time.Duration, 0),
			Labels: copyLabels(labels),
		}
		m.durations[key] = durationMetric
	}

	durationMetric.mu.Lock()
	durationMetric.Values = append(durationMetric.Values, duration)
	// Ограничиваем размер истории
	if len(durationMetric.Values) > MaxHistogramValues {
		durationMetric.Values = durationMetric.Values[len(durationMetric.Values)-1000:]
	}
	durationMetric.mu.Unlock()

	logger.Log.WithFields(map[string]any{
		"metric":      name,
		"labels":      labels,
		"duration_ms": duration.Milliseconds(),
	}).Debug("Duration recorded")
}

// GetCounters возвращает все счетчики
func (m *InMemoryMetrics) GetCounters() map[string]*Counter {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Counter)
	for k, v := range m.counters {
		v.mu.RLock()
		result[k] = &Counter{
			Value:  v.Value,
			Labels: copyLabels(v.Labels),
		}
		v.mu.RUnlock()
	}
	return result
}

// GetGauges возвращает все gauge метрики
func (m *InMemoryMetrics) GetGauges() map[string]*Gauge {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Gauge)
	for k, v := range m.gauges {
		v.mu.RLock()
		result[k] = &Gauge{
			Value:  v.Value,
			Labels: copyLabels(v.Labels),
		}
		v.mu.RUnlock()
	}
	return result
}

// GetHistograms возвращает все гистограммы
func (m *InMemoryMetrics) GetHistograms() map[string]*Histogram {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Histogram)
	for k, v := range m.histograms {
		v.mu.RLock()
		values := make([]float64, len(v.Values))
		copy(values, v.Values)
		result[k] = &Histogram{
			Values: values,
			Labels: copyLabels(v.Labels),
		}
		v.mu.RUnlock()
	}
	return result
}

// GetDurations возвращает все метрики времени выполнения
func (m *InMemoryMetrics) GetDurations() map[string]*Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Duration)
	for k, v := range m.durations {
		v.mu.RLock()
		values := make([]time.Duration, len(v.Values))
		copy(values, v.Values)
		result[k] = &Duration{
			Values: values,
			Labels: copyLabels(v.Labels),
		}
		v.mu.RUnlock()
	}
	return result
}

// Reset сбрасывает все метрики
func (m *InMemoryMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counters = make(map[string]*Counter)
	m.gauges = make(map[string]*Gauge)
	m.histograms = make(map[string]*Histogram)
	m.durations = make(map[string]*Duration)

	logger.Log.Info("All metrics reset")
}

// buildKey создает ключ для метрики с учетом labels
func (*InMemoryMetrics) buildKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	key := name
	for k, v := range labels {
		key += ":" + k + "=" + v
	}
	return key
}

// copyLabels создает копию labels
func copyLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}

	result := make(map[string]string, len(labels))
	for k, v := range labels {
		result[k] = v
	}
	return result
}

// NoOpMetrics реализует интерфейс MetricsInterface без выполнения операций
type NoOpMetrics struct{}

// NewNoOpMetrics создает no-op реализацию метрик
func NewNoOpMetrics() domain.MetricsInterface {
	return &NoOpMetrics{}
}

// IncrementCounter no-op реализация
func (*NoOpMetrics) IncrementCounter(_ string, _ map[string]string) {}

// SetGauge no-op реализация
func (*NoOpMetrics) SetGauge(_ string, _ float64, _ map[string]string) {}

// RecordHistogram no-op реализация
func (*NoOpMetrics) RecordHistogram(_ string, _ float64, _ map[string]string) {}

// RecordDuration no-op реализация
func (*NoOpMetrics) RecordDuration(_ string, _ time.Duration, _ map[string]string) {}

// MetricsCollector собирает и экспортирует метрики
type MetricsCollector struct {
	metrics domain.MetricsInterface
}

// NewMetricsCollector создает новый коллектор метрик
func NewMetricsCollector(metrics domain.MetricsInterface) *MetricsCollector {
	return &MetricsCollector{
		metrics: metrics,
	}
}

// CollectSystemMetrics собирает системные метрики
func (mc *MetricsCollector) CollectSystemMetrics() {
	// Собираем количество горутин
	numGoroutines := float64(runtime.NumGoroutine())
	mc.metrics.SetGauge("system_goroutines", numGoroutines, nil)

	// Собираем статистику памяти
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Память в мегабайтах для удобства
	const bytesToMB = 1024 * 1024

	mc.metrics.SetGauge("system_memory_alloc_mb", float64(memStats.Alloc)/bytesToMB, nil)
	mc.metrics.SetGauge("system_memory_sys_mb", float64(memStats.Sys)/bytesToMB, nil)
	mc.metrics.SetGauge("system_memory_heap_alloc_mb", float64(memStats.HeapAlloc)/bytesToMB, nil)
	mc.metrics.SetGauge("system_memory_heap_sys_mb", float64(memStats.HeapSys)/bytesToMB, nil)
	mc.metrics.SetGauge("system_gc_cycles", float64(memStats.NumGC), nil)

	// Собираем количество CPU
	numCPU := float64(runtime.NumCPU())
	mc.metrics.SetGauge("system_cpu_count", numCPU, nil)

	logger.Log.WithFields(map[string]any{
		"goroutines":   numGoroutines,
		"memory_alloc": float64(memStats.Alloc) / bytesToMB,
		"memory_sys":   float64(memStats.Sys) / bytesToMB,
		"gc_cycles":    memStats.NumGC,
		"cpu_count":    numCPU,
	}).Debug("System metrics collected")
}

// StartPeriodicCollection запускает периодический сбор метрик
func (mc *MetricsCollector) StartPeriodicCollection(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			mc.CollectSystemMetrics()
		}
	}()
}
