package platform

import "testing"

func TestLocalDeveloperAssetEmbeddingIsDeterministicAndAliasAware(t *testing.T) {
	t.Parallel()
	first := localDeveloperAssetEmbedding("Configure authentication and retry backoff")
	second := localDeveloperAssetEmbedding("Configure authentication and retry backoff")
	if len(first) != developerAssetEmbeddingDimensions || len(second) != len(first) {
		t.Fatalf("dimensions = %d, %d", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("embedding is not deterministic at %d", index)
		}
	}
	query := localDeveloperAssetEmbedding("auth credential")
	unrelated := localDeveloperAssetEmbedding("shipping address fields")
	if cosineForTest(first, query) <= cosineForTest(first, unrelated) {
		t.Fatal("retrieval aliases did not make auth query closer to authentication content")
	}
}

func cosineForTest(left, right []float32) float64 {
	var dot, leftNorm, rightNorm float64
	for index := range left {
		dot += float64(left[index] * right[index])
		leftNorm += float64(left[index] * left[index])
		rightNorm += float64(right[index] * right[index])
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (sqrtForTest(leftNorm) * sqrtForTest(rightNorm))
}

func sqrtForTest(value float64) float64 {
	guess := value
	for index := 0; index < 20; index++ {
		guess = (guess + value/guess) / 2
	}
	return guess
}
