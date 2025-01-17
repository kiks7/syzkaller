// Copyright 2021 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/syzkaller/prog"
)

// Stats encapsulates data for creating statistics related to the results
// of the verification process.
type Stats struct {
	// Calls stores statistics for all supported system calls.
	Calls               map[string]*CallStats
	TotalCallMismatches int64
	TotalProgs          int64
	ExecErrorProgs      int64
	FlakyProgs          int64
	MismatchingProgs    int64
	StartTime           time.Time
}

// CallStats stores information used to generate statistics for the
// system call.
type CallStats struct {
	// Name is the system call name.
	Name string
	// Mismatches stores the number of errno mismatches identified in the
	// verified programs for this system call.
	Mismatches int64
	// Occurrences is the number of times the system call appeared in a
	// verified program.
	Occurrences int64
	// States stores the kernel return state that caused mismatches.
	States map[ReturnState]bool
}

// MakeStats creates a stats object.
func MakeStats() *Stats {
	return &Stats{
		Calls: make(map[string]*CallStats),
	}
}

// SetSyscallMask initializes the allowed syscall list.
func (stats *Stats) SetSyscallMask(calls map[*prog.Syscall]bool) {
	stats.StartTime = time.Now()

	for syscall := range calls {
		stats.Calls[syscall.Name] = &CallStats{
			Name:   syscall.Name,
			States: make(map[ReturnState]bool)}
	}
}

// ReportGlobalStats creates a report with statistics about all the
// supported system calls for which errno mismatches were identified in
// the verified programs, shown in decreasing order.
func (stats *Stats) GetTextDescription(deltaTime float64) string {
	var result strings.Builder

	tc := stats.totalCallsExecuted()
	fmt.Fprintf(&result, "total number of mismatches / total number of calls "+
		"executed: %d / %d (%0.2f %%)\n\n", stats.TotalCallMismatches, tc, getPercentage(stats.TotalCallMismatches, tc))
	fmt.Fprintf(&result, "programs / minute: %0.2f\n\n", float64(stats.TotalProgs)/deltaTime)
	fmt.Fprintf(&result, "true mismatching programs: %d / total number of programs: %d (%0.2f %%)\n",
		stats.MismatchingProgs, stats.TotalProgs, getPercentage(stats.MismatchingProgs, stats.TotalProgs))
	fmt.Fprintf(&result, "flaky programs: %d / total number of programs: %d (%0.2f %%)\n\n",
		stats.FlakyProgs, stats.TotalProgs, getPercentage(stats.FlakyProgs, stats.TotalProgs))
	cs := stats.getOrderedStats()
	for _, c := range cs {
		fmt.Fprintf(&result, "%s\n", stats.getCallStatsTextDescription(c.Name))
	}

	return result.String()
}

// getCallStatsTextDescription creates a report with the current statistics for call.
func (stats *Stats) getCallStatsTextDescription(call string) string {
	syscallStat, ok := stats.Calls[call]
	if !ok {
		return ""
	}
	syscallName, mismatches, occurrences := syscallStat.Name, syscallStat.Mismatches, syscallStat.Occurrences
	return fmt.Sprintf("statistics for %s:\n"+
		"\t↳ mismatches of %s / occurrences of %s: %d / %d (%0.2f %%)\n"+
		"\t↳ mismatches of %s / total number of mismatches: "+
		"%d / %d (%0.2f %%)\n"+
		"\t↳ %d distinct states identified: %v\n", syscallName, syscallName, syscallName, mismatches, occurrences,
		getPercentage(mismatches, occurrences), syscallName, mismatches, stats.TotalCallMismatches,
		getPercentage(mismatches, stats.TotalCallMismatches), len(syscallStat.States), stats.getOrderedStates(syscallName))
}

func (stats *Stats) totalCallsExecuted() int64 {
	var t int64
	for _, cs := range stats.Calls {
		t += cs.Occurrences
	}
	return t
}

func (stats *Stats) getOrderedStats() []*CallStats {
	css := make([]*CallStats, 0)
	for _, cs := range stats.Calls {
		if cs.Mismatches > 0 {
			css = append(css, cs)
		}
	}

	sort.Slice(css, func(i, j int) bool {
		return getPercentage(css[i].Mismatches, css[i].Occurrences) > getPercentage(css[j].Mismatches, css[j].Occurrences)
	})

	return css
}

func (stats *Stats) getOrderedStates(call string) []string {
	states := stats.Calls[call].States
	ss := make([]string, 0, len(states))
	for s := range states {
		ss = append(ss, fmt.Sprintf("%q", s))
	}
	sort.Strings(ss)
	return ss
}

func getPercentage(value, total int64) float64 {
	return float64(value) / float64(total) * 100
}
