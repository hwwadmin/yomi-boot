// 相似度计算与门面 Score 映射。
//
// @author zwei

package vectordb

import "math"

// similarityScore 按度量计算门面 Score（越大越相似）
func similarityScore(metric Metric, query, vec []float32) float32 {
	switch metricOrDefault(metric) {
	case MetricL2:
		return l2ToScore(l2Distance(query, vec))
	case MetricIP:
		return dot(query, vec)
	default:
		return cosineSimilarity(query, vec)
	}
}

// cosineSimilarity 余弦相似度；零向量返回 0
func cosineSimilarity(a, b []float32) float32 {
	var dotSum, na, nb float32
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		dotSum += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dotSum / float32(math.Sqrt(float64(na))*math.Sqrt(float64(nb)))
}

// dot 内积
func dot(a, b []float32) float32 {
	var s float32
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		s += a[i] * b[i]
	}
	return s
}

// l2Distance 欧氏距离
func l2Distance(a, b []float32) float32 {
	var s float32
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		d := a[i] - b[i]
		s += d * d
	}
	return float32(math.Sqrt(float64(s)))
}

// l2ToScore 将 L2 距离映射为越大越相似
func l2ToScore(distance float32) float32 {
	return 1 / (1 + distance)
}

// engineScoreToHit 将引擎原始分映射为门面 Score
func engineScoreToHit(metric Metric, raw float32) float32 {
	if metricOrDefault(metric) == MetricL2 {
		return l2ToScore(raw)
	}
	return raw
}
