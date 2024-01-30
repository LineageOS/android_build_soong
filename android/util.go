// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import (
	"cmp"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// CopyOf returns a new slice that has the same contents as s.
func CopyOf[T any](s []T) []T {
	// If the input is nil, return nil and not an empty list
	if s == nil {
		return s
	}
	return append([]T{}, s...)
}

// Concat returns a new slice concatenated from the two input slices. It does not change the input
// slices.
func Concat[T any](s1, s2 []T) []T {
	res := make([]T, 0, len(s1)+len(s2))
	res = append(res, s1...)
	res = append(res, s2...)
	return res
}

// JoinPathsWithPrefix converts the paths to strings, prefixes them
// with prefix and then joins them separated by " ".
func JoinPathsWithPrefix(paths []Path, prefix string) string {
	strs := make([]string, len(paths))
	for i := range paths {
		strs[i] = paths[i].String()
	}
	return JoinWithPrefixAndSeparator(strs, prefix, " ")
}

// JoinWithPrefix prepends the prefix to each string in the list and
// returns them joined together with " " as separator.
func JoinWithPrefix(strs []string, prefix string) string {
	return JoinWithPrefixAndSeparator(strs, prefix, " ")
}

// JoinWithPrefixAndSeparator prepends the prefix to each string in the list and
// returns them joined together with the given separator.
func JoinWithPrefixAndSeparator(strs []string, prefix string, sep string) string {
	return JoinWithPrefixSuffixAndSeparator(strs, prefix, "", sep)
}

// JoinWithSuffixAndSeparator appends the suffix to each string in the list and
// returns them joined together with the given separator.
func JoinWithSuffixAndSeparator(strs []string, suffix string, sep string) string {
	return JoinWithPrefixSuffixAndSeparator(strs, "", suffix, sep)
}

// JoinWithPrefixSuffixAndSeparator appends the prefix/suffix to each string in the list and
// returns them joined together with the given separator.
func JoinWithPrefixSuffixAndSeparator(strs []string, prefix, suffix, sep string) string {
	if len(strs) == 0 {
		return ""
	}

	// Pre-calculate the length of the result
	length := 0
	for _, s := range strs {
		length += len(s)
	}
	length += (len(prefix)+len(suffix))*len(strs) + len(sep)*(len(strs)-1)

	var buf strings.Builder
	buf.Grow(length)
	buf.WriteString(prefix)
	buf.WriteString(strs[0])
	buf.WriteString(suffix)
	for i := 1; i < len(strs); i++ {
		buf.WriteString(sep)
		buf.WriteString(prefix)
		buf.WriteString(strs[i])
		buf.WriteString(suffix)
	}
	return buf.String()
}

// SortedStringKeys returns the keys of the given map in the ascending order.
//
// Deprecated: Use SortedKeys instead.
func SortedStringKeys[V any](m map[string]V) []string {
	return SortedKeys(m)
}

// SortedKeys returns the keys of the given map in the ascending order.
func SortedKeys[T cmp.Ordered, V any](m map[T]V) []T {
	if len(m) == 0 {
		return nil
	}
	ret := make([]T, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i] < ret[j]
	})
	return ret
}

// stringValues returns the values of the given string-valued map in randomized map order.
func stringValues(m interface{}) []string {
	v := reflect.ValueOf(m)
	if v.Kind() != reflect.Map {
		panic(fmt.Sprintf("%#v is not a map", m))
	}
	if v.Len() == 0 {
		return nil
	}
	iter := v.MapRange()
	s := make([]string, 0, v.Len())
	for iter.Next() {
		s = append(s, iter.Value().String())
	}
	return s
}

// SortedStringValues returns the values of the given string-valued map in the ascending order.
func SortedStringValues(m interface{}) []string {
	s := stringValues(m)
	sort.Strings(s)
	return s
}

// SortedUniqueStringValues returns the values of the given string-valued map in the ascending order
// with duplicates removed.
func SortedUniqueStringValues(m interface{}) []string {
	s := stringValues(m)
	return SortedUniqueStrings(s)
}

// IndexList returns the index of the first occurrence of the given string in the list or -1
func IndexList[T comparable](t T, list []T) int {
	for i, l := range list {
		if l == t {
			return i
		}
	}
	return -1
}

func InList[T comparable](t T, list []T) bool {
	return IndexList(t, list) != -1
}

func setFromList[T comparable](l []T) map[T]bool {
	m := make(map[T]bool, len(l))
	for _, t := range l {
		m[t] = true
	}
	return m
}

// ListSetDifference checks if the two lists contain the same elements. It returns
// a boolean which is true if there is a difference, and then returns lists of elements
// that are in l1 but not l2, and l2 but not l1.
func ListSetDifference[T comparable](l1, l2 []T) (bool, []T, []T) {
	listsDiffer := false
	diff1 := []T{}
	diff2 := []T{}
	m1 := setFromList(l1)
	m2 := setFromList(l2)
	for t := range m1 {
		if _, ok := m2[t]; !ok {
			diff1 = append(diff1, t)
			listsDiffer = true
		}
	}
	for t := range m2 {
		if _, ok := m1[t]; !ok {
			diff2 = append(diff2, t)
			listsDiffer = true
		}
	}
	return listsDiffer, diff1, diff2
}

// Returns true if the given string s is prefixed with any string in the given prefix list.
func HasAnyPrefix(s string, prefixList []string) bool {
	for _, prefix := range prefixList {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// Returns true if any string in the given list has the given substring.
func SubstringInList(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// Returns true if any string in the given list has the given prefix.
func PrefixInList(list []string, prefix string) bool {
	for _, s := range list {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// Returns true if any string in the given list has the given suffix.
func SuffixInList(list []string, suffix string) bool {
	for _, s := range list {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// IndexListPred returns the index of the element which in the given `list` satisfying the predicate, or -1 if there is no such element.
func IndexListPred(pred func(s string) bool, list []string) int {
	for i, l := range list {
		if pred(l) {
			return i
		}
	}

	return -1
}

// FilterList divides the string list into two lists: one with the strings belonging
// to the given filter list, and the other with the remaining ones
func FilterList(list []string, filter []string) (remainder []string, filtered []string) {
	// InList is O(n). May be worth using more efficient lookup for longer lists.
	for _, l := range list {
		if InList(l, filter) {
			filtered = append(filtered, l)
		} else {
			remainder = append(remainder, l)
		}
	}

	return
}

// FilterListPred returns the elements of the given list for which the predicate
// returns true. Order is kept.
func FilterListPred(list []string, pred func(s string) bool) (filtered []string) {
	for _, l := range list {
		if pred(l) {
			filtered = append(filtered, l)
		}
	}
	return
}

// RemoveListFromList removes the strings belonging to the filter list from the
// given list and returns the result
func RemoveListFromList(list []string, filter_out []string) (result []string) {
	result = make([]string, 0, len(list))
	for _, l := range list {
		if !InList(l, filter_out) {
			result = append(result, l)
		}
	}
	return
}

// RemoveFromList removes given string from the string list.
func RemoveFromList(s string, list []string) (bool, []string) {
	result := make([]string, 0, len(list))
	var removed bool
	for _, item := range list {
		if item != s {
			result = append(result, item)
		} else {
			removed = true
		}
	}
	return removed, result
}

// FirstUniqueStrings returns all unique elements of a slice of strings, keeping the first copy of
// each.  It does not modify the input slice.
func FirstUniqueStrings(list []string) []string {
	return firstUnique(list)
}

// firstUnique returns all unique elements of a slice, keeping the first copy of each.  It
// does not modify the input slice.
func firstUnique[T comparable](slice []T) []T {
	// Do not modify the input in-place, operate on a copy instead.
	slice = CopyOf(slice)
	return firstUniqueInPlace(slice)
}

// firstUniqueInPlace returns all unique elements of a slice, keeping the first copy of
// each.  It modifies the slice contents in place, and returns a subslice of the original
// slice.
func firstUniqueInPlace[T comparable](slice []T) []T {
	// 128 was chosen based on BenchmarkFirstUniqueStrings results.
	if len(slice) > 128 {
		return firstUniqueMap(slice)
	}
	return firstUniqueList(slice)
}

// firstUniqueList is an implementation of firstUnique using an O(N^2) list comparison to look for
// duplicates.
func firstUniqueList[T any](in []T) []T {
	writeIndex := 0
outer:
	for readIndex := 0; readIndex < len(in); readIndex++ {
		for compareIndex := 0; compareIndex < writeIndex; compareIndex++ {
			if interface{}(in[readIndex]) == interface{}(in[compareIndex]) {
				// The value at readIndex already exists somewhere in the output region
				// of the slice before writeIndex, skip it.
				continue outer
			}
		}
		if readIndex != writeIndex {
			in[writeIndex] = in[readIndex]
		}
		writeIndex++
	}
	return in[0:writeIndex]
}

// firstUniqueMap is an implementation of firstUnique using an O(N) hash set lookup to look for
// duplicates.
func firstUniqueMap[T comparable](in []T) []T {
	writeIndex := 0
	seen := make(map[T]bool, len(in))
	for readIndex := 0; readIndex < len(in); readIndex++ {
		if _, exists := seen[in[readIndex]]; exists {
			continue
		}
		seen[in[readIndex]] = true
		if readIndex != writeIndex {
			in[writeIndex] = in[readIndex]
		}
		writeIndex++
	}
	return in[0:writeIndex]
}

// ReverseSliceInPlace reverses the elements of a slice in place and returns it.
func ReverseSliceInPlace[T any](in []T) []T {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
	return in
}

// ReverseSlice returns a copy of a slice in reverse order.
func ReverseSlice[T any](in []T) []T {
	if in == nil {
		return in
	}
	out := make([]T, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[len(in)-1-i]
	}
	return out
}

// LastUniqueStrings returns all unique elements of a slice of strings, keeping the last copy of
// each.  It modifies the slice contents in place, and returns a subslice of the original slice.
func LastUniqueStrings(list []string) []string {
	totalSkip := 0
	for i := len(list) - 1; i >= totalSkip; i-- {
		skip := 0
		for j := i - 1; j >= totalSkip; j-- {
			if list[i] == list[j] {
				skip++
			} else {
				list[j+skip] = list[j]
			}
		}
		totalSkip += skip
	}
	return list[totalSkip:]
}

// SortedUniqueStrings returns what the name says
func SortedUniqueStrings(list []string) []string {
	// FirstUniqueStrings creates a copy of `list`, so the input remains untouched.
	unique := FirstUniqueStrings(list)
	sort.Strings(unique)
	return unique
}

// checkCalledFromInit panics if a Go package's init function is not on the
// call stack.
func checkCalledFromInit() {
	for skip := 3; ; skip++ {
		_, funcName, ok := callerName(skip)
		if !ok {
			panic("not called from an init func")
		}

		if funcName == "init" || strings.HasPrefix(funcName, "init·") ||
			strings.HasPrefix(funcName, "init.") {
			return
		}
	}
}

// A regex to find a package path within a function name. It finds the shortest string that is
// followed by '.' and doesn't have any '/'s left.
var pkgPathRe = regexp.MustCompile(`^(.*?)\.([^/]+)$`)

// callerName returns the package path and function name of the calling
// function.  The skip argument has the same meaning as the skip argument of
// runtime.Callers.
func callerName(skip int) (pkgPath, funcName string, ok bool) {
	var pc [1]uintptr
	n := runtime.Callers(skip+1, pc[:])
	if n != 1 {
		return "", "", false
	}

	f := runtime.FuncForPC(pc[0]).Name()
	s := pkgPathRe.FindStringSubmatch(f)
	if len(s) < 3 {
		panic(fmt.Errorf("failed to extract package path and function name from %q", f))
	}

	return s[1], s[2], true
}

// GetNumericSdkVersion removes the first occurrence of system_ in a string,
// which is assumed to be something like "system_1.2.3"
func GetNumericSdkVersion(v string) string {
	return strings.Replace(v, "system_", "", 1)
}

// copied from build/kati/strutil.go
func substPattern(pat, repl, str string) string {
	ps := strings.SplitN(pat, "%", 2)
	if len(ps) != 2 {
		if str == pat {
			return repl
		}
		return str
	}
	in := str
	trimmed := str
	if ps[0] != "" {
		trimmed = strings.TrimPrefix(in, ps[0])
		if trimmed == in {
			return str
		}
	}
	in = trimmed
	if ps[1] != "" {
		trimmed = strings.TrimSuffix(in, ps[1])
		if trimmed == in {
			return str
		}
	}

	rs := strings.SplitN(repl, "%", 2)
	if len(rs) != 2 {
		return repl
	}
	return rs[0] + trimmed + rs[1]
}

// copied from build/kati/strutil.go
func matchPattern(pat, str string) bool {
	i := strings.IndexByte(pat, '%')
	if i < 0 {
		return pat == str
	}
	return strings.HasPrefix(str, pat[:i]) && strings.HasSuffix(str, pat[i+1:])
}

var shlibVersionPattern = regexp.MustCompile("(?:\\.\\d+(?:svn)?)+")

// splitFileExt splits a file name into root, suffix and ext. root stands for the file name without
// the file extension and the version number (e.g. "libexample"). suffix stands for the
// concatenation of the file extension and the version number (e.g. ".so.1.0"). ext stands for the
// file extension after the version numbers are trimmed (e.g. ".so").
func SplitFileExt(name string) (string, string, string) {
	// Extract and trim the shared lib version number if the file name ends with dot digits.
	suffix := ""
	matches := shlibVersionPattern.FindAllStringIndex(name, -1)
	if len(matches) > 0 {
		lastMatch := matches[len(matches)-1]
		if lastMatch[1] == len(name) {
			suffix = name[lastMatch[0]:lastMatch[1]]
			name = name[0:lastMatch[0]]
		}
	}

	// Extract the file name root and the file extension.
	ext := filepath.Ext(name)
	root := strings.TrimSuffix(name, ext)
	suffix = ext + suffix

	return root, suffix, ext
}

// ShardPaths takes a Paths, and returns a slice of Paths where each one has at most shardSize paths.
func ShardPaths(paths Paths, shardSize int) []Paths {
	if len(paths) == 0 {
		return nil
	}
	ret := make([]Paths, 0, (len(paths)+shardSize-1)/shardSize)
	for len(paths) > shardSize {
		ret = append(ret, paths[0:shardSize])
		paths = paths[shardSize:]
	}
	if len(paths) > 0 {
		ret = append(ret, paths)
	}
	return ret
}

// ShardString takes a string and returns a slice of strings where the length of each one is
// at most shardSize.
func ShardString(s string, shardSize int) []string {
	if len(s) == 0 {
		return nil
	}
	ret := make([]string, 0, (len(s)+shardSize-1)/shardSize)
	for len(s) > shardSize {
		ret = append(ret, s[0:shardSize])
		s = s[shardSize:]
	}
	if len(s) > 0 {
		ret = append(ret, s)
	}
	return ret
}

// ShardStrings takes a slice of strings, and returns a slice of slices of strings where each one has at most shardSize
// elements.
func ShardStrings(s []string, shardSize int) [][]string {
	if len(s) == 0 {
		return nil
	}
	ret := make([][]string, 0, (len(s)+shardSize-1)/shardSize)
	for len(s) > shardSize {
		ret = append(ret, s[0:shardSize])
		s = s[shardSize:]
	}
	if len(s) > 0 {
		ret = append(ret, s)
	}
	return ret
}

// CheckDuplicate checks if there are duplicates in given string list.
// If there are, it returns first such duplicate and true.
func CheckDuplicate(values []string) (duplicate string, found bool) {
	seen := make(map[string]string)
	for _, v := range values {
		if duplicate, found = seen[v]; found {
			return duplicate, true
		}
		seen[v] = v
	}
	return "", false
}

func AddToStringSet(set map[string]bool, items []string) {
	for _, item := range items {
		set[item] = true
	}
}

// SyncMap is a wrapper around sync.Map that provides type safety via generics.
type SyncMap[K comparable, V any] struct {
	sync.Map
}

// Load returns the value stored in the map for a key, or the zero value if no
// value is present.
// The ok result indicates whether value was found in the map.
func (m *SyncMap[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.Map.Load(key)
	if !ok {
		return *new(V), false
	}
	return v.(V), true
}

// Store sets the value for a key.
func (m *SyncMap[K, V]) Store(key K, value V) {
	m.Map.Store(key, value)
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *SyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	v, loaded := m.Map.LoadOrStore(key, value)
	return v.(V), loaded
}
