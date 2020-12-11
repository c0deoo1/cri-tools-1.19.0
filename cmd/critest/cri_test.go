/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/gomega"

	_ "github.com/kubernetes-sigs/cri-tools/pkg/benchmark"
	"github.com/kubernetes-sigs/cri-tools/pkg/common"
	"github.com/kubernetes-sigs/cri-tools/pkg/framework"
	_ "github.com/kubernetes-sigs/cri-tools/pkg/validate"
	versionconst "github.com/kubernetes-sigs/cri-tools/pkg/version"
)

const (
	parallelFlag  = "parallel"
	benchmarkFlag = "benchmark"
	versionFlag   = "version"
)

var (
	letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	isBenchMark = flag.Bool(benchmarkFlag, false, "Run benchmarks instead of validation tests")
	parallel    = flag.Int(parallelFlag, 1, "The number of parallel test nodes to run (default 1)")
	version     = flag.Bool(versionFlag, false, "Display version of critest")
)

func init() {
	framework.RegisterFlags()
	rand.Seed(time.Now().UnixNano())
	getConfigFromFile()
}

// Load server configuration from file and use each config settings if that
// option is not set in the CLI
func getConfigFromFile() {
	var configFromFile *common.ServerConfiguration

	currentPath, _ := os.Getwd()
	configFromFile, _ = common.GetServerConfigFromFile(framework.TestContext.ConfigPath, currentPath)

	if configFromFile != nil {
		// Command line flags take precedence over config file.
		if !isFlagSet("runtime-endpoint") && configFromFile.RuntimeEndpoint != "" {
			framework.TestContext.RuntimeServiceAddr = configFromFile.RuntimeEndpoint
		}
		if !isFlagSet("image-endpoint") && configFromFile.ImageEndpoint != "" {
			framework.TestContext.ImageServiceAddr = configFromFile.ImageEndpoint
		}
	}
}

func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// runTestSuite runs cri validation tests and benchmark tests.
func runTestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)

	reporter := []ginkgo.Reporter{}
	if framework.TestContext.ReportDir != "" {
		if err := os.MkdirAll(framework.TestContext.ReportDir, 0755); err != nil {
			t.Errorf("Failed creating report directory: %v", err)
		}

		reporter = append(reporter, reporters.NewJUnitReporter(path.Join(framework.TestContext.ReportDir, fmt.Sprintf("junit_%v.xml", framework.TestContext.ReportPrefix))))
	}

	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "CRI validation", reporter)
}

func generateTempTestName() (string, error) {
	suffix := make([]byte, 10)
	for i := range suffix {
		suffix[i] = letterBytes[rand.Intn(len(letterBytes))]
	}

	dir, err := ioutil.TempDir("", "cri-test")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "critest-"+string(suffix)+".test"), nil
}

func runParallelTestSuite(t *testing.T) {
	criPath, err := os.Executable()
	if err != nil {
		t.Fatalf("Failed to lookup path of critest: %v", err)
	}
	t.Logf("critest path: %s", criPath)

	tempFileName, err := generateTempTestName()
	if err != nil {
		t.Fatalf("Failed to generate temp test name: %v", err)
	}
	err = os.Symlink(criPath, tempFileName)
	if err != nil {
		t.Fatalf("Failed to lookup path of critest: %v", err)
	}
	defer os.Remove(tempFileName)

	ginkgoArgs := []string{fmt.Sprintf("-nodes=%d", *parallel)}
	var testArgs []string
	flag.Visit(func(f *flag.Flag) {
		if strings.HasPrefix(f.Name, "ginkgo.") {
			flagName := strings.TrimPrefix(f.Name, "ginkgo.")
			ginkgoArgs = append(ginkgoArgs, fmt.Sprintf("-%s=%s", flagName, f.Value.String()))
			return
		}
		if f.Name == parallelFlag || f.Name == benchmarkFlag {
			return
		}
		testArgs = append(testArgs, fmt.Sprintf("-%s=%s", f.Name, f.Value.String()))
	})
	var args []string
	args = append(args, ginkgoArgs...)
	args = append(args, tempFileName, "--")
	args = append(args, testArgs...)

	cmd := exec.Command("ginkgo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to run tests in parallel: %v", err)
	}
}

func TestCRISuite(t *testing.T) {
	fmt.Printf("critest version: %s\n", versionconst.Version)

	if *version {
		// print version only and exit
		return
	}

	if *isBenchMark {
		flag.Set("ginkgo.focus", "benchmark")
		flag.Set("ginkgo.succinct", "true")
	} else {
		// Skip benchmark measurements for validation tests.
		flag.Set("ginkgo.skipMeasurements", "true")
	}
	if *parallel > 1 {
		runParallelTestSuite(t)
	} else {
		runTestSuite(t)
	}
}
