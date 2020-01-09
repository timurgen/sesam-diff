package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultBranch = "master"

func main() {
	var pathToSource = flag.String("path", "", "path to sesam config, must be a GIT repo")
	var sesamNode = flag.String("node", "", "Sesam node name")
	var jwt = flag.String("jwt", "", "JWT token to access Sesam node")
	var branchToCheck = flag.String("branch", defaultBranch, "GIT branch to check against node")
	var outFile = flag.String("o", "", "Output file name")
	flag.Parse()

	if *pathToSource == "" {
		log.Fatal("Path to Sesam config must be provided")
	}

	if *sesamNode == "" {
		log.Fatal("Sesam node must be provided")
	}

	if *jwt == "" {
		log.Fatal("JWT token to access Sesam node must be provided")
	}

	err := os.Chdir(*pathToSource)
	if err != nil {
		log.Fatalf("Couldn't change working directory due to: %s", err)
	}

	if *branchToCheck == defaultBranch {
		log.Printf("Default branch (%q) will be used\n", defaultBranch)
	}

	gitCheckCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	var out bytes.Buffer
	gitCheckCmd.Stdout = &out

	err = gitCheckCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	if "true\n" != out.String() {
		log.Fatalf("Directory %q is not a GIT repo", *pathToSource)
	}
	out.Reset()

	gitBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	gitBranchCmd.Stdout = &out
	err = gitBranchCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	var currentBranch = string(bytes.TrimRight(out.Bytes(), "\n"))
	out.Reset()

	var newBranch = fmt.Sprintf("temporary-differ-branch-%d", time.Now().Unix())

	if currentBranch != *branchToCheck {
		log.Printf("current branch (%q) differs from branch to check (%q)", currentBranch, *branchToCheck)
		log.Printf("switch to branch %q", *branchToCheck)

		gitSwitchBranchCmd := exec.Command("git", "checkout", *branchToCheck)
		gitSwitchBranchCmd.Stdout = &out
		err = gitSwitchBranchCmd.Run()
		if err != nil {
			log.Fatal(err)
		}
		out.Reset()
	}

	gitCheckoutNewbranchCmd := exec.Command("git", "checkout", "-b", newBranch)
	gitCheckoutNewbranchCmd.Stdout = &out
	err = gitCheckoutNewbranchCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	out.Reset()

	client := &http.Client{}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/api/config", *sesamNode), nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Add("Accept", "application/zip")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", *jwt))

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode >= 400 {
		log.Fatal(resp.Status)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		log.Fatal(err)
	}

	for _, zipFile := range zipReader.File {
		fmt.Println("Reading file:", zipFile.Name)
		unzippedFileBytes, err := readZipFile(zipFile)
		if err != nil {
			log.Fatal(err)
		}
		workDir, _ := os.Getwd()
		err = ioutil.WriteFile(filepath.Clean(fmt.Sprintf("%s/%s", workDir, zipFile.Name)), unzippedFileBytes, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

	gitAddCmd := exec.Command("git", "add", ".")
	gitAddCmd.Stdout = &out
	err = gitAddCmd.Run()
	if err != nil {
		log.Println(out.String())
		log.Fatal(err)
	}
	out.Reset()

	gitCommitCmd := exec.Command("git", "commit", "-m", "test commit")
	gitCommitCmd.Stdout = &out
	err = gitCommitCmd.Run()
	if err != nil && !strings.Contains(out.String(), "working tree clean") {
		log.Println(out.String())
		log.Fatal(err)
	}
	out.Reset()

	gitDiffCmd := exec.Command("git", "diff", *branchToCheck, newBranch)
	gitDiffCmd.Stdout = &out

	err = gitDiffCmd.Run()
	if err != nil {
		log.Println(out.String())
		log.Fatal(err)
	}
	var diff = out.String()
	out.Reset()

	gitCheckoutOriginalBranchCmd := exec.Command("git", "checkout", currentBranch)
	gitCheckoutOriginalBranchCmd.Stdout = &out
	err = gitCheckoutOriginalBranchCmd.Run()
	if err != nil {
		log.Println(out.String())
		log.Fatal(err)
	}
	out.Reset()

	gitDeleteTempBranchCmd := exec.Command("git", "branch", "-D", newBranch)
	gitDeleteTempBranchCmd.Stdout = &out
	err = gitDeleteTempBranchCmd.Run()
	if err != nil {
		log.Println(out.String())
		log.Fatal(err)
	}
	out.Reset()

	if *outFile != "" {
		ioutil.WriteFile(*outFile, []byte(diff), 0644)
		return
	}
	fmt.Println(diff)

}

func readZipFile(zf *zip.File) ([]byte, error) {
	f, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}
