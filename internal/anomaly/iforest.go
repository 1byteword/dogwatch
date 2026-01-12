package anomaly

import (
	"math"
	"math/rand"
	"sync"
)

// IsolationForest implements the Isolation Forest algorithm for anomaly detection
// This is a pure Go implementation optimized for speed
type IsolationForest struct {
	trees      []*iTree
	numTrees   int
	sampleSize int
	trained    bool
	avgPathLen float64
	mu         sync.RWMutex
	rng        *rand.Rand
}

// iTree represents a single isolation tree
type iTree struct {
	root *iNode
}

// iNode represents a node in an isolation tree
type iNode struct {
	left       *iNode
	right      *iNode
	splitAttr  int
	splitValue float64
	size       int // Number of samples in this node (for external nodes)
	isExternal bool
}

// NewIsolationForest creates a new isolation forest
func NewIsolationForest(numTrees, sampleSize int) *IsolationForest {
	return &IsolationForest{
		numTrees:   numTrees,
		sampleSize: sampleSize,
		rng:        rand.New(rand.NewSource(42)),
	}
}

// Fit trains the isolation forest on the given data
func (f *IsolationForest) Fit(data [][]float64) {
	if len(data) == 0 {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Compute average path length for normalization
	n := float64(len(data))
	if n > 2 {
		f.avgPathLen = 2*(math.Log(n-1)+0.5772156649) - (2*(n-1))/n
	} else if n == 2 {
		f.avgPathLen = 1
	} else {
		f.avgPathLen = 0
	}

	// Build trees in parallel
	f.trees = make([]*iTree, f.numTrees)
	var wg sync.WaitGroup

	for i := 0; i < f.numTrees; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Sample data
			sample := f.subsample(data)

			// Build tree
			maxHeight := int(math.Ceil(math.Log2(float64(len(sample)))))
			f.trees[idx] = &iTree{
				root: f.buildTree(sample, 0, maxHeight),
			}
		}(i)
	}

	wg.Wait()
	f.trained = true
}

// subsample randomly samples data
func (f *IsolationForest) subsample(data [][]float64) [][]float64 {
	n := len(data)
	size := f.sampleSize
	if size > n {
		size = n
	}

	// Fisher-Yates shuffle for first 'size' elements
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}

	for i := 0; i < size; i++ {
		j := i + f.rng.Intn(n-i)
		indices[i], indices[j] = indices[j], indices[i]
	}

	sample := make([][]float64, size)
	for i := 0; i < size; i++ {
		sample[i] = data[indices[i]]
	}

	return sample
}

// buildTree recursively builds an isolation tree
func (f *IsolationForest) buildTree(data [][]float64, height, maxHeight int) *iNode {
	n := len(data)

	// External node conditions
	if height >= maxHeight || n <= 1 {
		return &iNode{
			isExternal: true,
			size:       n,
		}
	}

	// Select random attribute
	numAttrs := len(data[0])
	attr := f.rng.Intn(numAttrs)

	// Find min/max for this attribute
	minVal, maxVal := data[0][attr], data[0][attr]
	for _, row := range data[1:] {
		if row[attr] < minVal {
			minVal = row[attr]
		}
		if row[attr] > maxVal {
			maxVal = row[attr]
		}
	}

	// If all values are the same, create external node
	if minVal == maxVal {
		return &iNode{
			isExternal: true,
			size:       n,
		}
	}

	// Random split value
	splitVal := minVal + f.rng.Float64()*(maxVal-minVal)

	// Partition data
	var left, right [][]float64
	for _, row := range data {
		if row[attr] < splitVal {
			left = append(left, row)
		} else {
			right = append(right, row)
		}
	}

	// Handle edge cases
	if len(left) == 0 || len(right) == 0 {
		return &iNode{
			isExternal: true,
			size:       n,
		}
	}

	return &iNode{
		splitAttr:  attr,
		splitValue: splitVal,
		left:       f.buildTree(left, height+1, maxHeight),
		right:      f.buildTree(right, height+1, maxHeight),
	}
}

// Score computes the anomaly score for a single point (0-1, higher = more anomalous)
func (f *IsolationForest) Score(point []float64) float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if !f.trained || len(f.trees) == 0 {
		return 0.5 // Default score when not trained
	}

	// Average path length across all trees
	var totalPathLen float64
	for _, tree := range f.trees {
		totalPathLen += f.pathLength(point, tree.root, 0)
	}
	avgPath := totalPathLen / float64(len(f.trees))

	// Anomaly score: 2^(-avgPath/c(n))
	// Where c(n) is the average path length for unsuccessful search in BST
	score := math.Pow(2, -avgPath/f.avgPathLen)

	return score
}

// pathLength computes the path length for a point in a tree
func (f *IsolationForest) pathLength(point []float64, node *iNode, currentLen float64) float64 {
	if node == nil || node.isExternal {
		if node != nil && node.size > 1 {
			// Add average path length for remaining samples
			n := float64(node.size)
			return currentLen + 2*(math.Log(n-1)+0.5772156649) - (2*(n-1))/n
		}
		return currentLen
	}

	// Bounds check
	if node.splitAttr >= len(point) {
		return currentLen
	}

	if point[node.splitAttr] < node.splitValue {
		return f.pathLength(point, node.left, currentLen+1)
	}
	return f.pathLength(point, node.right, currentLen+1)
}

// ScoreBatch computes anomaly scores for multiple points efficiently
func (f *IsolationForest) ScoreBatch(points [][]float64) []float64 {
	scores := make([]float64, len(points))

	// Process in parallel for large batches
	if len(points) > 100 {
		var wg sync.WaitGroup
		batchSize := (len(points) + 3) / 4 // 4 goroutines

		for b := 0; b < 4; b++ {
			start := b * batchSize
			end := start + batchSize
			if end > len(points) {
				end = len(points)
			}
			if start >= end {
				continue
			}

			wg.Add(1)
			go func(s, e int) {
				defer wg.Done()
				for i := s; i < e; i++ {
					scores[i] = f.Score(points[i])
				}
			}(start, end)
		}
		wg.Wait()
	} else {
		for i, p := range points {
			scores[i] = f.Score(p)
		}
	}

	return scores
}

// Update incrementally updates the forest with new data (reservoir sampling)
func (f *IsolationForest) Update(point []float64) {
	// For online learning, we could implement reservoir sampling
	// and periodically retrain. For now, this is a placeholder.
}
