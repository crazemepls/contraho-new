package main

import (
	"fmt"
	"strings"
)

func listApp(host string, credential string, lengthProject int, authMode int, listMode int) interface{} {

	if listMode == 1 {
		displayJob("list app", "start")
		go displayLoading(loadingCh)
	}
	var applicationListforProject []string
	var applicationList []AppList
	for pageIndex := 1; pageIndex <= lengthProject; pageIndex++ {
		raw := projectSearchApi(host, "APP", pageSize, pageIndex, credential, authMode)

		var structured ProjectSearchPage
		if err := dataParse(raw, &structured); err != nil {
			fmt.Println(err)
		}

		for _, component := range structured.Components {
			if listMode == 0 {
				applicationListforProject = append(applicationListforProject, component.Key)
			} else {
				applicationList = append(applicationList, AppList{Key: component.Key, Name: component.Name})
			}
		}
	}

	if listMode == 0 {
		return applicationListforProject
	}

	loadingCh <- true
	displayJob("list app", "end")
	return applicationList
}

func languageofApp(appList []AppList, host string, credential string, authMode int) []AppList {
	displayJob("obtain language of project", "start")
	go displayLoading(loadingCh)
	// const empty = `-`

	for indexApp := range appList {
		lengthProjectOfAppPage := applicationProjectSearchApiLength(host, appList[indexApp].Key, credential, authMode)
		var listedProjectOnApp []string
		listedProjectOnApp = listProjectofApplication(host, listedProjectOnApp, appList[indexApp].Key,
			lengthProjectOfAppPage, credential, authMode)
		var languageofProjectonApp []string

		for indexProject := range listedProjectOnApp {
			// raw := httpRequest(host+projectBranchesApi+projectList[index].Key, credential)
			raw := navigationComponentApi(host, listedProjectOnApp[indexProject], "", credential, authMode)
			var structured NavigationComponent
			err := dataParse(raw, &structured)
			handleErr(err)
			qualityProfilesKeys := make([]string, len(structured.QualityProfiles))
			for indexKey, profile := range structured.QualityProfiles {
				qualityProfilesKeys[indexKey] = profile.Key
			}
			// fmt.Println(qualityProfilesKeys)
			// time.Sleep(10 * time.Second)
			qualityProfilesNames := make([]string, len(qualityProfilesKeys)) //
			for indexKey, qualityProfileKey := range qualityProfilesKeys {
				qualityProfilesNames[indexKey] = languageOfProfile(host, qualityProfileKey, credential, authMode)
			}
			languageofProjectonApp = append(languageofProjectonApp, qualityProfilesNames...)

		}
		languageofProjectonApp = removeDuplicates(languageofProjectonApp)
		// // fmt.Println(qualityProfilesKey)

		// qualityProfilesNames := make([]string, len(qualityProfilesKeys)) //
		// for indexKey, qualityProfileKey := range qualityProfilesKeys {
		// 	raw := qualityProfilesShowApi(host, qualityProfileKey, credential, authMode)
		// 	var structured QualityProfilesShow
		// 	err := dataParse(raw, &structured)
		// 	handleErr(err)
		// 	qualityProfilesNames[indexKey] = structured.Profile.LanguageName
		// }
		appList[indexApp].Language = strings.Join(languageofProjectonApp, ", ")

	}
	loadingCh <- true
	displayJob("obtain language of project", "end")
	return appList
}

func removeDuplicates(input []string) []string {
	// Create a map to store unique values
	unique := make(map[string]bool)

	// Create a new slice to store unique values
	result := []string{}

	// Iterate over the input slice
	for _, val := range input {
		// If the value is not already in the unique map, add it to the result slice
		if !unique[val] {
			result = append(result, val)
			unique[val] = true
		}
	}

	return result
}
