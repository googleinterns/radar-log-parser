package report

import (
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	SpecificProcess     map[string]string
	IssuesGeneralFields struct {
		Number      string
		Details     string
		Timestamp   string
		Log_level   string
		OtherFields map[string]string
	}
	Issues          map[string]Issue
	Priority        map[string]int
	ImportantEvents map[string]string
}

type ConfigInterface struct {
	SpecificProcess     map[string]string `yaml:"SpecificProcess"`
	IssuesGeneralFields struct {
		Number      string            `yaml:"Number"`
		Details     string            `yaml:"Details"`
		Timestamp   string            `yaml:"Timestamp"`
		Log_level   string            `yaml:"LogLevel"`
		OtherFields map[string]string `yaml:"OtherFields"`
	} `yaml:"IssuesGeneralFields"`
	Issues          map[string]interface{} `yaml:"Issues"`
	Priority        map[string]int         `yaml:"Priority"`
	ImportantEvents map[string]string      `yaml:"ImportantEvents"`
}
type Issue struct {
	specific_process  map[string]string
	regex             string
	detailing_mode    string
	grouping          string
	additional_fields map[string]string
}
type GroupedStruct struct {
	Group_names   []string
	Group_content map[string][][]string
	Group_count   map[string][]int
}
type AnalysisDetails struct {
	FileName        string
	RawLog          string
	SpecificProcess map[string]string
	Header          []string
	OrderedIssues   []string
	Issues          map[string]map[string]string
	Platform        string
}
type FullDetails struct {
	Analysis_details AnalysisDetails
	GroupedIssues    map[string]GroupedStruct
	NonGroupedIssues map[string]map[string]bool
	ImportantEvents  map[int]string
}

func AnalyseLog(w http.ResponseWriter, r *http.Request, project_id string, region_id string, fullLogDetails *FullDetails, cfgFile *Config) error {
	fContent, fName, cfgName, bucket, err := uploadLogFile(w, r, project_id, region_id)
	if err != nil {
		return err
	}
	fullLogDetails.Analysis_details = AnalysisDetails{}
	//Set the selected platform
	fullLogDetails.Analysis_details.Platform = bucket
	err = extractConfig(cfgName, bucket, cfgFile)
	if err != nil {
		return err
	}
	fullLogDetails.GroupedIssues = make(map[string]GroupedStruct)
	fullLogDetails.NonGroupedIssues = make(map[string]map[string]bool)
	fullLogDetails.Analysis_details.FileName = *fName
	fullLogDetails.Analysis_details.RawLog = fContent
	fullLogDetails.Analysis_details.SpecificProcess = make(map[string]string)
	spec_proc_map := fullLogDetails.Analysis_details.SpecificProcess
	setSpecProcessLogs(cfgFile, fContent, spec_proc_map)
	//Fill the header with general fields
	headerMap := map[string]bool{"Issue": true, "Number": true, "Details": true, "Timestamp": true, "LogLevel": true}
	for field, _ := range cfgFile.IssuesGeneralFields.OtherFields {
		headerMap[field] = true
	}
	fullLogDetails.Analysis_details.Issues = make(map[string]map[string]string)
	issues_map := fullLogDetails.Analysis_details.Issues
	grp_issues := fullLogDetails.GroupedIssues
	ngrp_issues := fullLogDetails.NonGroupedIssues
	getIssueDetails(cfgFile, fContent, headerMap, issues_map, spec_proc_map, grp_issues, ngrp_issues)
	fullLogDetails.Analysis_details.OrderedIssues = make([]string, len(cfgFile.Issues), len(cfgFile.Issues))
	sortIssue(cfgFile, fullLogDetails.Analysis_details.OrderedIssues)
	fullLogDetails.Analysis_details.Header = fillHeader(headerMap)
	return nil
}
func sortIssue(cfgFile *Config, issues []string) {
	index := 0
	for k := range cfgFile.Issues {
		issues[index] = k
		index++
	}
	sort.Slice(issues, func(i, j int) bool {
		return cfgFile.Priority[issues[i]] > cfgFile.Priority[issues[j]]
	})
}
func fillHeader(headerMap map[string]bool) []string {
	header := make([]string, 0, len(headerMap))
	header = append(header, "Issue", "Number", "Details", "Timestamp", "LogLevel")
	for _, field := range header {
		headerMap[field] = false
	}
	for field, value := range headerMap {
		if value {
			header = append(header, field)
		}
	}
	return header
}
func setSpecProcessLogs(cfgFile *Config, fContent string, spec_proc_map map[string]string) {
	/*var waitGroup sync.WaitGroup
	var mutex sync.Mutex
	waitGroup.Add(len(cfgFile.SpecificProcess))
	for proc, proc_rgx := range cfgFile.SpecificProcess {
		go func(proc string, proc_rgx string) {
			proc_rgx_comp, err := regexp.Compile(proc_rgx)
			if err != nil {
				waitGroup.Done()
				return
			}
			proc_content := proc_rgx_comp.FindAllString(fContent, -1)
			if len(proc_content) > 1 {
				mutex.Lock()
				spec_proc_map[proc] = strings.Join(proc_content, "\n")
				mutex.Unlock()
			}
			waitGroup.Done()
		}(proc, proc_rgx)
	}
	waitGroup.Wait()*/
	for proc, proc_rgx := range cfgFile.SpecificProcess {
		proc_rgx_comp, err := regexp.Compile(proc_rgx)
		if err != nil {
			continue
		}
		proc_content := proc_rgx_comp.FindAllString(fContent, -1)
		if len(proc_content) > 1 {
			spec_proc_map[proc] = strings.Join(proc_content, "\n")
		}
	}

}
func getIssueDetails(cfgFile *Config, fContent string, headerMap map[string]bool, issues_map map[string]map[string]string, spec_proc_map map[string]string, grp_issues map[string]GroupedStruct, ngrp_issues map[string]map[string]bool) {
	/*specific_proc_content := make(map[string]string)
	var wg sync.WaitGroup
	var mutex sync.Mutex
	var map_mutex sync.Mutex
	var spec_mutex sync.Mutex
	wg.Add(len(cfgFile.Issues))
	for issue_name, issue := range cfgFile.Issues {
		go func(issue_name string, issue Issue) {
			map_mutex.Lock()
			issues_map[issue_name] = make(map[string]string)
			map_mutex.Unlock()
			//Filter the logs belonging to the issue specific process
			issueContent := ""
			for proc, proc_rgx := range issue.specific_process {
				proc_issue, ok := spec_proc_map[proc]
				if !ok {
					spec_mutex.Lock()
					proc_issue, ok := specific_proc_content[proc]
					spec_mutex.Unlock()
					if !ok {
						proc_rgx_comp, err := regexp.Compile(proc_rgx)
						if err != nil {
							continue
						}
						raw_proc_issue := proc_rgx_comp.FindAllString(fContent, -1)
						proc_issue = strings.Join(raw_proc_issue, "\n")
						spec_mutex.Lock()
						specific_proc_content[proc] = proc_issue
						spec_mutex.Unlock()
					}
				}
				issueContent += proc_issue
				issueContent += "\n"
			}
			mutex.Lock()
			if issue.detailing_mode == "group" {
				groupedDetails := GroupedStruct{}
				groupedDetails.Group_content = make(map[string][][]string)
				groupedDetails.Group_count = make(map[string][]int)
				groupedDetails.Group_names = []string{}
				groupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name, issues_map, groupedDetails.Group_content, groupedDetails.Group_count, &groupedDetails.Group_names)
				grp_issues[issue_name] = groupedDetails
			} else {
				nongroupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name, issues_map, ngrp_issues)
			}
			mutex.Unlock()
			wg.Done()
		}(issue_name, issue)
	}
	wg.Wait()*/

	specific_proc_content := make(map[string]string)
	for issue_name, issue := range cfgFile.Issues {
		issues_map[issue_name] = make(map[string]string)
		//Filter the logs belonging to the issue specific process
		issueContent := ""
		for proc, proc_rgx := range issue.specific_process {
			proc_issue, ok := spec_proc_map[proc]
			if !ok {
				proc_issue, ok := specific_proc_content[proc]
				if !ok {
					proc_rgx_comp, err := regexp.Compile(proc_rgx)
					if err != nil {
						continue
					}
					raw_proc_issue := proc_rgx_comp.FindAllString(fContent, -1)
					proc_issue = strings.Join(raw_proc_issue, "\n")
					specific_proc_content[proc] = proc_issue
				}
			}
			issueContent += proc_issue
			issueContent += "\n"
		}

		if issue.detailing_mode == "group" {
			groupedDetails := GroupedStruct{}
			groupedDetails.Group_content = make(map[string][][]string)
			groupedDetails.Group_count = make(map[string][]int)
			groupedDetails.Group_names = []string{}
			groupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name, issues_map, groupedDetails.Group_content, groupedDetails.Group_count, &groupedDetails.Group_names)
			grp_issues[issue_name] = groupedDetails
		} else {
			nongroupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name, issues_map, ngrp_issues)
		}

	}
}
func groupIssueDetails(issue Issue, cfgFile *Config, headerMap map[string]bool, issueContent string, issue_name string, issues_map map[string]map[string]string, group_content map[string][][]string, group_count map[string][]int, group_names *[]string) {
	group_rgx, err := regexp.Compile(issue.grouping)
	if err != nil {
		return
	}
	names := group_rgx.SubexpNames()
	names_p := &names
	*group_names = *names_p
	last_matches, issues_count := fillGroupDetails(group_content, group_count, issueContent, group_rgx)
	issue_map := issues_map[issue_name]
	issue_map["Number"] += strconv.Itoa(issues_count)

	timestampRegex, _ := regexp.Compile(cfgFile.IssuesGeneralFields.Timestamp)
	match := timestampRegex.FindStringSubmatch(last_matches)
	if len(match) > 0 {
		issue_map["Timestamp"] = match[0]
	}

	log_rgx, err := regexp.Compile(cfgFile.IssuesGeneralFields.Log_level)
	if err != nil {
		return
	}
	match = log_rgx.FindStringSubmatch(last_matches)
	if len(match) > 1 {
		issue_map["LogLevel"] = match[1]
	}
	for field, field_rgx := range cfgFile.IssuesGeneralFields.OtherFields {
		field_issue := issues_map[issue_name]
		setFieldContent(field_rgx, issueContent, field, field_issue)
	}

	for field, field_rgx := range issue.additional_fields {
		field_issue := issues_map[issue_name]
		setFieldContent(field_rgx, issueContent, field, field_issue)
		headerMap[field] = true
	}
}
func fillGroupDetails(group_content map[string][][]string, group_count map[string][]int, issueContent string, group_rgx *regexp.Regexp) (string, int) {
	last_matches := ""
	issueContentSlice := strings.Split(issueContent, "\n")
	for _, log := range issueContentSlice {
		matches := group_rgx.FindStringSubmatch(log)
		if len(matches) > 2 {
			last_matches = log
			if group_content[matches[1]] == nil {
				group_content[matches[1]] = [][]string{}
				group_count[matches[1]] = []int{}
			}
			exist := false
			for index, grp_detail := range group_content[matches[1]] {
				if reflect.DeepEqual(grp_detail, matches[2:]) {
					group_count[matches[1]][index] += 1
					exist = true
					break
				}
			}
			if !exist {
				group_count[matches[1]] = append(group_count[matches[1]], 1)
				group_content[matches[1]] = append(group_content[matches[1]], matches[2:])
			}
		}
	}
	issues_count := 0
	for _, numSlice := range group_count {
		for _, num := range numSlice {
			issues_count += num
		}
	}
	return last_matches, issues_count
}
func nongroupIssueDetails(issue Issue, cfgFile *Config, headerMap map[string]bool, issueContent string, issue_name string, issues_map map[string]map[string]string, ngroup_map map[string]map[string]bool) {
	issue_rgx, err := regexp.Compile(issue.regex)
	if err != nil {
		return
	}
	filter_logs := issue_rgx.FindAllString(issueContent, -1)
	filter_logs_map := make(map[string]bool)
	for _, filter_log := range filter_logs {
		filter_logs_map[filter_log] = true
	}
	ngroup_map[issue_name] = filter_logs_map
	issue_map := issues_map[issue_name]
	issue_map["Number"] = strconv.Itoa(len(filter_logs))
	issueContent = strings.Join(filter_logs, "\n")
	if len(filter_logs) > 0 {
		log_rgx, err := regexp.Compile(cfgFile.IssuesGeneralFields.Log_level)
		if err != nil {
			return
		}
		match := log_rgx.FindStringSubmatch(filter_logs[0])
		if len(match) > 1 {
			issue_map["LogLevel"] = match[1]
		}
		for field, field_rgx := range cfgFile.IssuesGeneralFields.OtherFields {
			field_issue := issues_map[issue_name]
			setFieldContent(field_rgx, issueContent, field, field_issue)
		}
		for field, field_rgx := range issue.additional_fields {
			field_issue := issues_map[issue_name]
			setFieldContent(field_rgx, issueContent, field, field_issue)
			headerMap[field] = true
		}
		timestampRegex, _ := regexp.Compile(cfgFile.IssuesGeneralFields.Timestamp)
		match = timestampRegex.FindStringSubmatch(filter_logs[len(filter_logs)-1])
		if len(match) > 0 {
			issue_map["Timestamp"] = match[0]
		}
	}
}
func getFieldContent(field_rgx string, issueContent string) string {
	field_rgx_comp, err := regexp.Compile(field_rgx)
	if err != nil {
		return ""
	}
	match := field_rgx_comp.FindAllString(issueContent, -1)
	fieldContent := strconv.Itoa(len(match)) + " :  " + strings.Join(match, "\n")
	return fieldContent
}
func setFieldContent(field_rgx string, issueContent string, field string, field_issue map[string]string) {
	field_content := getFieldContent(field_rgx, issueContent)
	field_issue[field] = field_content
}
