package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

func httpRequest(url string, credential string, mode int) (data []byte, statusCode int) {
	const maxAttempts = 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error creating HTTP request:", err)
			os.Exit(1)
		}

		switch mode {
		case 1:
			req.Header.Add("Authorization", credential)
		case 0:
			req.SetBasicAuth(credential, "")
		}

		resp, err := httpClient.Do(req)
		if err == nil {
			data, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			statusCode = resp.StatusCode
		}

		// Retry on network errors, 429 and 5xx; give up after maxAttempts.
		if err != nil || statusCode == http.StatusTooManyRequests || statusCode >= 500 {
			if attempt < maxAttempts {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error requesting", url, ":", err)
				os.Exit(1)
			}
		}
		break
	}

	switch statusCode {
	case http.StatusUnauthorized:
		fmt.Fprintln(os.Stderr, "Error: authentication failed (401). Check --token or --username/--password.")
		os.Exit(1)
	case http.StatusForbidden:
		fmt.Fprintln(os.Stderr, "Warning: access denied (403) for", url)
	}

	return data, statusCode
}

func projectSearchApi(host string, qualifiers string, size int, pageNumber int, credential string, mode int) (data []byte) {
	const (
		projectsSearch = "/api/projects/search"
	)
	queryParams := url.Values{}
	queryParams.Add("qualifiers", qualifiers)
	queryParams.Add("ps", fmt.Sprintf("%d", size))
	queryParams.Add("p", fmt.Sprintf("%d", pageNumber))
	encodedQuery := queryParams.Encode()
	fullPath := host + projectsSearch + "?" + encodedQuery

	data, _ = httpRequest(fullPath, credential, mode)
	return data
}

func projectBranchesListApi(host string, projectKey string, credential string, mode int) (data []byte) {
	const (
		projectBranchesList = "/api/project_branches/list"
	)
	queryParams := url.Values{}
	queryParams.Add("project", projectKey)
	encodedQuery := queryParams.Encode()
	fullPath := host + projectBranchesList + "?" + encodedQuery

	data, _ = httpRequest(fullPath, credential, mode)

	return data
}

func measuresComponentApi(host string, projectKey string, branch string, metricKeys string, credential string, mode int) (data []byte) {
	const (
		measuresComponent = "/api/measures/component"
	)
	queryParams := url.Values{}
	queryParams.Add("metricKeys", metricKeys)
	queryParams.Add("component", projectKey)
	queryParams.Add("branch", branch)
	encodedQuery := queryParams.Encode()
	fullPath := host + measuresComponent + "?" + encodedQuery
	// fmt.Println(fullPath)

	data, _ = httpRequest(fullPath, credential, mode)

	return data

}

func projectAnalysesSearchApi(host string, size int, pageNumber int, projectKey string, branch string, credential string, mode int) (data []byte) {
	const (
		projectAnalysesSearch = "/api/project_analyses/search"
	)
	queryParams := url.Values{}
	queryParams.Add("ps", fmt.Sprintf("%d", size))
	queryParams.Add("p", fmt.Sprintf("%d", pageNumber))
	queryParams.Add("branch", branch)
	queryParams.Add("project", projectKey)
	encodedQuery := queryParams.Encode()
	fullPath := host + projectAnalysesSearch + "?" + encodedQuery

	data, _ = httpRequest(fullPath, credential, mode)

	return data

}

func permissionUsersApi(host string, projectKey string, credential string, mode int) (data []byte) {
	const (
		permissionUsers = "/api/permissions/users"
	)

	queryParams := url.Values{}
	queryParams.Add("projectKey", projectKey)

	encodedQuery := queryParams.Encode()
	fullPath := host + permissionUsers + "?" + encodedQuery
	// fmt.Println(fullPath)
	data, _ = httpRequest(fullPath, credential, mode)

	return data

}

func applicationsSearchApi(host string, size int, pageNumber int, applicationKey string, credential string, mode int) (data []byte) {
	const (
		applicationsSearch = "/api/applications/search_projects"
	)
	queryParams := url.Values{}
	queryParams.Add("application", applicationKey)
	queryParams.Add("ps", fmt.Sprintf("%d", size))
	queryParams.Add("p", fmt.Sprintf("%d", pageNumber))
	encodedQuery := queryParams.Encode()
	fullPath := host + applicationsSearch + "?" + encodedQuery
	data, _ = httpRequest(fullPath, credential, mode)
	return data

}

func navigationGlobalApi(host string, credential string, mode int) (data []byte) {
	const (
		navigationGlobal = "/api/navigation/global"
	)
	fullPath := host + navigationGlobal
	// fmt.Println(credential)
	data, _ = httpRequest(fullPath, credential, mode)
	return data
}

func qualityGatesGetByProjectApi(host string, projectKey string, credential string, mode int) (data []byte) {
	const (
		qualityGatesGetByProject = "/api/qualitygates/get_by_project"
	)

	queryParams := url.Values{}
	queryParams.Add("project", projectKey)
	encodedQuery := queryParams.Encode()
	fullPath := host + qualityGatesGetByProject + "?" + encodedQuery
	data, _ = httpRequest(fullPath, credential, mode)
	return data
}

func navigationComponentApi(host string, projectKey string, branch string, credential string, mode int) (data []byte) {
	const (
		navigationComponent = "/api/navigation/component"
	)

	queryParams := url.Values{}
	queryParams.Set("component", projectKey)
	if branch != "" {
		queryParams.Set("branch", branch)
	}
	encodedQuery := queryParams.Encode()
	fullPath := host + navigationComponent + "?" + encodedQuery
	data, _ = httpRequest(fullPath, credential, mode)
	return data
}

func qualityProfilesShowApi(host string, qualityProfileKey string, credential string, mode int) (data []byte) {
	const (
		qualityProfilesShow = "/api/qualityprofiles/show"
	)

	queryParams := url.Values{}
	queryParams.Set("key", qualityProfileKey)
	encodedQuery := queryParams.Encode()
	fullPath := host + qualityProfilesShow + "?" + encodedQuery
	data, _ = httpRequest(fullPath, credential, mode)
	return data
}

func measuresSearchHistoryApi(host string, projectKey string, branch string, metrics string, dateFrom string, dateTo string,
	credential string, mode int) (data []byte) {
	const (
		measuresSearchHistory = "/api/measures/search_history"
	)
	queryParams := url.Values{}
	queryParams.Set("component", projectKey)
	queryParams.Set("branch", branch)
	queryParams.Set("metrics", metrics)
	queryParams.Set("from", dateFrom)
	queryParams.Set("to", dateTo)
	encodedQuery := queryParams.Encode()
	fullPath := host + measuresSearchHistory + "?" + encodedQuery
	data, _ = httpRequest(fullPath, credential, mode)
	return data
}

func applicationsShowApi(host string, applicationKey string, branch string, credential string, mode int) (data []byte, statusCode int) {
	const (
		measuresSearchHistory = "/api/applications/show"
	)
	queryParams := url.Values{}
	queryParams.Set("application", applicationKey)
	queryParams.Set("branch", branch)
	encodedQuery := queryParams.Encode()
	fullPath := host + measuresSearchHistory + "?" + encodedQuery
	// fmt.Println(fullPath)
	data, statusCode = httpRequest(fullPath, credential, mode)
	return data, statusCode
}
