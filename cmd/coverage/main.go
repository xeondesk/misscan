// Command coverage reports detection coverage gaps in the rule registry:
// rules per framework/provider/service, duplicate AVD IDs, and — per
// compliance spec — checks that have no registered rule.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/khulnasoft-lab/misscan/internal/rules"
	"github.com/khulnasoft-lab/misscan/pkg/framework"
	dftypes "github.com/khulnasoft-lab/misscan/pkg/types"
	ruleTypes "github.com/khulnasoft-lab/misscan/pkg/types/rules"
	"github.com/khulnasoft-lab/tunnel-audit/specs"

	// Register every bundled check (tunnel-audit imports).
	_ "github.com/khulnasoft-lab/misscan/pkg/rego"
	_ "github.com/khulnasoft-lab/misscan/pkg/rules"
)

// knownSpecs lists the compliance spec IDs embedded in tunnel-audit.
var knownSpecs = []string{
	"aws-cis-1.2",
	"aws-cis-1.4",
	"docker-cis",
	"k8s-cis",
	"k8s-nsa",
	"k8s-pss-baseline",
	"k8s-pss-restricted",
}

func main() {
	specFlag := flag.String("spec", "", "compliance spec ID to audit (e.g. aws-cis-1.4); empty audits registry + all known specs")
	flag.Parse()

	registered := rules.GetFrameworkRules(framework.ALL)

	if *specFlag != "" {
		if !reportSpec(*specFlag, registered) {
			os.Exit(1)
		}
		return
	}
	reportRegistry(registered)
	missing := 0
	for _, s := range knownSpecs {
		if !reportSpec(s, registered) {
			missing++
		}
	}
	if missing > 0 {
		fmt.Printf("\n%d spec(s) have uncovered checks\n", missing)
	}
}

// reportRegistry prints rule counts grouped by provider and service, plus
// duplicate AVD ID detection.
func reportRegistry(registered []ruleTypes.RegisteredRule) {
	fmt.Printf("total rules: %d\n\n", len(registered))

	type svcKey struct{ provider, service string }
	counts := map[svcKey]int{}
	avdIDs := map[string]int{}
	for _, rr := range registered {
		r := rr.GetRule()
		counts[svcKey{string(r.Provider), r.Service}]++
		if r.AVDID != "" {
			avdIDs[r.AVDID]++
		}
	}

	keys := make([]svcKey, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].provider != keys[j].provider {
			return keys[i].provider < keys[j].provider
		}
		return keys[i].service < keys[j].service
	})

	fmt.Println("provider/service coverage:")
	for _, k := range keys {
		fmt.Printf("  %-12s %-30s %d\n", k.provider, k.service, counts[k])
	}

	var dups []string
	for id, n := range avdIDs {
		if n > 1 {
			dups = append(dups, fmt.Sprintf("%s (x%d)", id, n))
		}
	}
	if len(dups) > 0 {
		sort.Strings(dups)
		fmt.Printf("\nduplicate AVD IDs:\n  %s\n", strings.Join(dups, "\n  "))
	}
}

// reportSpec diffs a compliance spec's expected check IDs against the
// registered rules. Returns false when the spec is unknown or has gaps.
func reportSpec(spec string, registered []ruleTypes.RegisteredRule) bool {
	content := getSpecContent(spec)
	if content == "" {
		fmt.Printf("spec %q: not found\n\n", spec)
		return false
	}

	var specDoc dftypes.ComplianceSpec
	if err := yaml.Unmarshal([]byte(content), &specDoc); err != nil {
		fmt.Printf("spec %q: parse error: %v\n\n", spec, err)
		return false
	}

	registeredIDs := map[string]bool{}
	for _, rr := range registered {
		if id := rr.GetRule().AVDID; id != "" {
			registeredIDs[strings.ToLower(id)] = true
		}
	}

	var missing []string
	var matched int
	for _, ctrl := range specDoc.Spec.Controls {
		for _, chk := range ctrl.Checks {
			if registeredIDs[strings.ToLower(chk.ID)] {
				matched++
			} else {
				missing = append(missing, fmt.Sprintf("%s (%s)", chk.ID, ctrl.Name))
			}
		}
	}

	fmt.Printf("spec %s (%s): %d checks, %d covered, %d missing\n",
		specDoc.Spec.ID, specDoc.Spec.Title, matched+len(missing), matched, len(missing))
	for _, m := range missing {
		fmt.Printf("  MISSING %s\n", m)
	}
	fmt.Println()
	return len(missing) == 0
}

// getSpecContent resolves a spec by ID via the tunnel-audit specs loader.
func getSpecContent(spec string) string {
	return specs.GetSpec(spec)
}
