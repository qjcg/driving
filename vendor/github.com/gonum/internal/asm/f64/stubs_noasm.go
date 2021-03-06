// Copyright ©2016 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//+build !amd64 noasm appengine

package f64

import "math"

func AbsSum(x []float64) (sum float64) {
	for _, v := range x {
		sum += math.Abs(v)
	}
	return sum
}

func AbsSumInc(x []float64, n, incX int) (sum float64) {
	for i := 0; i < n*incX; i += incX {
		sum += math.Abs(x[i])
	}
	return sum
}

func Add(dst, s []float64) {
	for i, v := range s {
		dst[i] += v
	}
}

func AddConst(alpha float64, x []float64) {
	for i := range x {
		x[i] += alpha
	}
}

func CumSum(dst, s []float64) []float64 {
	if len(s) == 0 {
		return dst
	}
	dst[0] = s[0]
	for i, v := range s[1:] {
		dst[i+1] = dst[i] + v
	}
	return dst
}

func CumProd(dst, s []float64) []float64 {
	if len(s) == 0 {
		return dst
	}
	dst[0] = s[0]
	for i, v := range s[1:] {
		dst[i+1] = dst[i] * v
	}
	return dst
}

func Div(dst, s []float64) {
	for i, v := range s {
		dst[i] /= v
	}
}

func DivTo(dst, s, t []float64) []float64 {
	for i, v := range s {
		dst[i] = v / t[i]
	}
	return dst
}

func L1Norm(s, t []float64) float64 {
	var norm float64
	for i, v := range s {
		norm += math.Abs(t[i] - v)
	}
	return norm
}

func LinfNorm(s, t []float64) float64 {
	var norm float64
	if len(s) == 0 {
		return 0
	}
	norm = math.Abs(t[0] - s[0])
	for i, v := range s[1:] {
		absDiff := math.Abs(t[i+1] - v)
		if absDiff > norm || math.IsNaN(norm) {
			norm = absDiff
		}
	}
	return norm
}
