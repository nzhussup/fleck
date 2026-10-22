package registry

import (
	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/detector/rules/cfg001"
	"github.com/nzhussup/fleck/internal/detector/rules/cfg002"
	"github.com/nzhussup/fleck/internal/detector/rules/cfg003"
	"github.com/nzhussup/fleck/internal/detector/rules/cfg004"
	"github.com/nzhussup/fleck/internal/detector/rules/cfg005"
)

func All() []detector.Detector {
	return []detector.Detector{
		cfg001.New(),
		cfg002.New(cfg002.DefaultThreshold),
		cfg003.New(),
		cfg004.New(),
		cfg005.New(),
	}
}

func Metadata() []detector.Metadata {
	rules := All()
	metadata := make([]detector.Metadata, 0, len(rules))
	for _, rule := range rules {
		metadata = append(metadata, rule.Metadata())
	}
	return metadata
}
