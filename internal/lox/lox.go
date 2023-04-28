package lox

import (
	"github.com/samber/lo"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
)

func MapToPredicate[K comparable, V any](m map[K]V) func(K) bool {
	return func(v K) bool {
		_, ok := m[v]
		return ok
	}
}
func SliceToPredicateBy[K comparable, V any](s []V, f func(V) K) func(K) bool {
	return MapToPredicate(SliceToSetBy(s, f))
}

func SliceToPredicate[V comparable](s []V) func(V) bool {
	return MapToPredicate(SliceToSet(s))
}

func SliceToSet[V comparable](collection []V) map[V]struct{} {
	return SliceToSetBy(collection, Identity[V])
}

func SliceToSetBy[K comparable, V any](collection []V, iteratee func(item V) K) map[K]struct{} {
	return lo.Associate(collection, func(item V) (K, struct{}) {
		return iteratee(item), struct{}{}
	})
}

func Identity[T any](v T) T {
	return v
}

func IfOrEmpty[T any](condition bool, result T) T {
	return lo.Ternary(condition, result, lo.Empty[T]())
}
func IfOrEmptyF[T any](condition bool, f func() T) T {
	return lo.TernaryF(condition, f, lo.Empty[T])
}

func Compose[T1, T2, R any](f func(T2) R, g func(T1) T2) func(T1) R {
	return func(v T1) R {
		return f(g(v))
	}
}

func Not[T any](f func(T) bool) func(T) bool {
	return func(v T) bool {
		return !f(v)
	}
}

func IgnoreIndex[T1, T2, R any](f func(T1) R) func(T1, T2) R {
	return IgnoreSecond[T1, T2, R](f)
}

func IgnoreSecond[T1, T2, R any](f func(T1) R) func(T1, T2) R {
	return func(v T1, _ T2) R {
		return f(v)
	}
}

func SortBy[T any, R constraints.Ordered](s []T, f func(T) R) {
	slices.SortFunc(s, func(a, b T) bool {
		return f(a) < f(b)
	})

}

func OnlyEmptyBy[T, R comparable](collection []T, iteratee func(item T) R) []T {
	return FilterWithoutIndex(collection, Compose[T, R, bool](lo.IsEmpty[R], iteratee))
}

func WithoutEmptyBy[T, R comparable](collection []T, iteratee func(item T) R) []T {
	return FilterWithoutIndex(collection, Compose[T, R, bool](lo.IsNotEmpty[R], iteratee))
}

func FilterWithoutIndex[V any](collection []V, predicate func(item V) bool) []V {
	return lo.Filter(collection, IgnoreSecond[V, int, bool](predicate))
}
func MapWithoutIndex[T, R any](collection []T, iteratee func(item T) R) []R {
	return lo.Map(collection, IgnoreSecond[T, int, R](iteratee))
}
