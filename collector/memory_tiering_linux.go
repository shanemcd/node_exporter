// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !nomemory_tiering

package collector

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
)

const memoryTieringSubsystem = "memory_tiering"

type memoryTieringCollector struct {
	fs     sysfs.FS
	logger *slog.Logger
}

func init() {
	registerCollector("memory_tiering", defaultDisabled, NewMemoryTieringCollector)
}

// NewMemoryTieringCollector returns a new Collector exposing the Linux
// memory-tiering topology: which NUMA nodes belong to each memory tier.
func NewMemoryTieringCollector(logger *slog.Logger) (Collector, error) {
	fs, err := sysfs.NewFS(*sysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sysfs: %w", err)
	}
	return &memoryTieringCollector{
		fs:     fs,
		logger: logger,
	}, nil
}

// Update calls the memory tiering metrics function.
func (c *memoryTieringCollector) Update(ch chan<- prometheus.Metric) error {
	tiering, err := c.fs.MemoryTiering()
	if errors.Is(err, os.ErrNotExist) {
		// The kernel does not expose memory tiers; nothing to report.
		return ErrNoData
	}
	if err != nil {
		return err
	}

	nodeDesc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, memoryTieringSubsystem, "node_tier_info"),
		"Memory-tier membership of a NUMA node (always 1). The tier id is the kernel's memory tier number; tiers with a lower abstract distance are faster.",
		[]string{"tier", "numa_node"}, nil,
	)
	for tierID, tier := range tiering {
		for _, node := range tier.Nodes {
			ch <- prometheus.MustNewConstMetric(
				nodeDesc, prometheus.GaugeValue, 1,
				fmt.Sprintf("%d", tierID), fmt.Sprintf("%d", node),
			)
		}
	}

	distanceDesc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, memoryTieringSubsystem, "tier_abstract_distance"),
		"Abstract distance of a memory tier as reported by the kernel; higher values are slower tiers (DRAM is the reference at 200).",
		[]string{"tier"}, nil,
	)
	for tierID, tier := range tiering {
		if !tier.HasAbstractDistance {
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			distanceDesc, prometheus.GaugeValue, float64(tier.AbstractDistance),
			fmt.Sprintf("%d", tierID),
		)
	}

	return nil
}
