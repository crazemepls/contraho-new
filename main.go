package main

import (
	"fmt"
	"os"
	"time"
)

const (
	emptyString = ""
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: contraho <subcommand> [options]")
		os.Exit(1)
	}

	subcommand := os.Args[1]

	switch subcommand {
	case "project", "proj", "p":
		projectSearch()
	case "application", "app", "a":
		appSearch()
	default:
		fmt.Println("Invalid subcommand:", subcommand)
		os.Exit(1)
	}
}

func projectSearch() {
	startTime := time.Now()

	host, credential, authMode, fileOutput, pagingOutput, otherOptions := arguments(0)

	lengthProjectPage := projectSearchApiLength(*host, credential, "TRK", authMode)

	bureau, _ := otherOptions["bureau"].(string)

	var projectList []ProjectSearchList
	switch {
	case otherOptions["unlistedApp"] == true:
		projectList = project(*host, credential, authMode, lengthProjectPage, 0, emptyString, bureau)
	case otherOptions["listedApp"] == true:
		projectList = project(*host, credential, authMode, lengthProjectPage, 1, emptyString, bureau)
	case otherOptions["app"] != "":
		projectList = project(*host, credential, authMode, lengthProjectPage, 1, otherOptions["app"].(string), bureau)
	default:
		projectList = project(*host, credential, authMode, lengthProjectPage, -1, emptyString, bureau)
	}
	if bureau != "" && len(projectList) == 0 {
		fmt.Printf("No project key starts with %q (the match is case-sensitive).\n", bureau)
		os.Exit(1)
	}
	if *fileOutput != "" {
		createCSVFile(*fileOutput, startTime, projectList)
	} else {
		printStructTable(projectList, startTime, *pagingOutput, "Key", "Name", "Branch", "Loc", "Owner")

		// printStructAsTable(projectList, []string{"Key", "Name", "Branch", "Loc", "Owner"})
	}

}

func project(host string, credential string, authMode int, lengthProjectPage int, filterMode int, appName string, bureau string) []ProjectSearchList {
	list := listProject(host, credential, lengthProjectPage, authMode)
	// Filter first, so the per-project API calls below only run for matching projects.
	list = filterByKeyPrefix(list, bureau)
	if len(list) == 0 {
		return list
	}
	list = projectFiltering(list, host, credential, filterMode, authMode, appName)
	list = branchDetailOfProjects(list, host, credential, authMode).([]ProjectSearchList)
	list = qualityGateofProject(list, host, credential, authMode)
	list = languageofProject(list, host, credential, authMode)
	list = ownerProject(list, host, credential, authMode).([]ProjectSearchList)
	return metricProject(list, host, credential, authMode).([]ProjectSearchList)
}

func appSearch() {
	startTime := time.Now()

	host, credential, authMode, fileOutput, pagingOutput, _ := arguments(1)

	lengthAppPage := projectSearchApiLength(*host, credential, "APP", authMode)

	appListInterface := listApp(*host, credential, lengthAppPage, authMode, 1)

	// Type assertion to convert the interface to []AppList
	appList, ok := appListInterface.([]AppList)
	if !ok {
		// Handle the case where the returned value is not []AppList
		fmt.Println("Error: Unexpected return type from listApp")
		return
	}
	appList = languageofApp(appList, *host, credential, authMode)
	appList = ownerProject(appList, *host, credential, authMode).([]AppList)
	appList = branchDetailOfProjects(appList, *host, credential, authMode).([]AppList)
	appList = metricProject(appList, *host, credential, authMode).([]AppList)

	if *fileOutput != "" {
		createCSVFile(*fileOutput, startTime, appList)
	} else {
		printStructTable(appList, startTime, *pagingOutput, "Key", "Name", "Loc")
	}
}
