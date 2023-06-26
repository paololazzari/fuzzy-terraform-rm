package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/ktr0731/go-fuzzyfinder"
	"golang.org/x/exp/slices"
)

type Resource struct {
	Name     string
	Filename string
	Line     string
}

// get all terraform resources available for removal
func getTerraformResourcesToRemove(working_dir string) []map[string]string {
	var cmd strings.Builder
	cmd.WriteString("terraform state list")
	stdout, _, err := shellout(cmd.String(), true)
	if err != nil {
		fmt.Printf("No terraform state found\n")
		os.Exit(1)
	}
	stateResources := strings.Split(stdout, "\n")
	module, _ := tfconfig.LoadModule(working_dir)
	managedResources := (*module).ManagedResources
	resources := []map[string]string{}
	for _, managedResource := range managedResources {
		k := managedResource.Type
		v := managedResource.Name
		r := k + "." + v
		if slices.Contains(stateResources, r) {
			resource := make(map[string]string)
			resource["Name"] = r
			resource["Filename"] = filepath.Base(managedResource.Pos.Filename)
			resource["Line"] = strconv.Itoa(managedResource.Pos.Line)
			resources = append(resources, resource)
		}
	}
	return resources
}

// get the current working directory
func getDir() string {
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return pwd
}

// build the fuzzyfinder menu
func fuzzyMenu(resources []Resource) []string {
	resourcesToRemove := []string{}
	idx, err := fuzzyfinder.FindMulti(
		resources,
		func(i int) string {
			return resources[i].Name
		},
		fuzzyfinder.WithPreviewWindow(func(i, w, h int) string {
			if i == -1 {
				return ""
			}
			return fmt.Sprintf("Terraform resource: %s\nSource code file: %s\nLine: %s\n",
				resources[i].Name,
				resources[i].Filename,
				resources[i].Line,
			)
		}))
	if err != nil {
		log.Fatal(err)
	}
	for _, i := range idx {
		resourcesToRemove = append(resourcesToRemove, resources[i].Name)
	}
	return resourcesToRemove
}

// execute the given command in either bash or powershell depending on the detected os
func shellout(command string, silent bool) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := &exec.Cmd{}
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-command", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if silent != true {
		fmt.Println(cmd.Stdout)
		fmt.Println(cmd.Stderr)
	}
	return stdout.String(), stderr.String(), err
}

func main() {

	forceDestroy := flag.Bool("force", false, "If true, do not prompt for confirmation when removing a resource from the state")
	flag.Parse()

	workingDir := getDir()
	resourcesAvailableForRemoval := getTerraformResourcesToRemove(workingDir)

	if len(resourcesAvailableForRemoval) == 0 {
		fmt.Printf("No resources available for removal were found\n")
		os.Exit(1)
	}

	resources := []Resource{}
	for _, resource := range resourcesAvailableForRemoval {
		resources = append(resources, Resource{
			resource["Name"], resource["Filename"], resource["Line"],
		})
	}
	resourcesToRemove := fuzzyMenu(resources)

	reader := bufio.NewReader(os.Stdin)
	for _, resourceToRemove := range resourcesToRemove {
		input := ""

		for {
			if *forceDestroy == false {
				fmt.Printf("Remove %s from terraform state? [y/n]\n", resourceToRemove)
				input, _ = reader.ReadString('\n')
				input = strings.TrimSpace(input)
			} else {
				input = "y"
			}
			if input == "y" {
				var cmd strings.Builder
				tfImportCommand := "terraform state rm " + resourceToRemove
				cmd.WriteString(tfImportCommand)
				fmt.Printf("Executing: %s\n", cmd.String())
				shellout(cmd.String(), false)
				break
			} else if input == "n" {
				break
			} else {
				fmt.Printf("%s\n", "Invalid selection. Please select [y/n]")
			}
		}
	}
}
