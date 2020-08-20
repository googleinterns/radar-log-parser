package report

import (
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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

var (
	FullLogDetails = FullDetails{}
	CfgFile        = Config{}
)

func AnalyseLog(w http.ResponseWriter, r *http.Request, project_id string, region_id string) error {
	fContent, fName, cfgName, bucket, err := uploadLogFile(w, r, project_id, region_id)
	if err != nil {
		return err
	}
	FullLogDetails.Analysis_details = AnalysisDetails{}
	//Set the selected platform
	FullLogDetails.Analysis_details.Platform = bucket
	cfgFile, err := extractConfig(cfgName, bucket)
	if err != nil {
		return err
	}
	CfgFile = *cfgFile
	FullLogDetails.GroupedIssues = make(map[string]GroupedStruct)
	FullLogDetails.NonGroupedIssues = make(map[string]map[string]bool)
	FullLogDetails.Analysis_details.FileName = *fName
	FullLogDetails.Analysis_details.RawLog = fContent
	getSpecProcessLogs(cfgFile, fContent)
	//Fill the header with general fields
	headerMap := map[string]bool{"Issue": true, "Number": true, "Details": true, "Timestamp": true, "LogLevel": true}
	for field, _ := range cfgFile.IssuesGeneralFields.OtherFields {
		headerMap[field] = true
	}
	getIssueDetails(cfgFile, fContent, headerMap)
	FullLogDetails.Analysis_details.OrderedIssues = sortIssue(cfgFile)
	FullLogDetails.Analysis_details.Header = fillHeader(headerMap)
	return nil
}
func sortIssue(cfgFile *Config) []string {
	issues := make([]string, 0, len(cfgFile.Issues))
	for k := range cfgFile.Issues {
		issues = append(issues, k)
	}
	sort.Slice(issues, func(i, j int) bool {
		return cfgFile.Priority[issues[i]] > cfgFile.Priority[issues[j]]
	})
	return issues
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
func getSpecProcessLogs(cfgFile *Config, fContent string) {
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(cfgFile.SpecificProcess))
	FullLogDetails.Analysis_details.SpecificProcess = make(map[string]string)
	for proc, proc_rgx := range cfgFile.SpecificProcess {
		go func(proc string, proc_rgx string) {
			proc_rgx_comp, err := regexp.Compile(proc_rgx)
			if err != nil {
				waitGroup.Done()
				return
			}
			proc_content := proc_rgx_comp.FindAllString(fContent, -1)
			if len(proc_content) > 1 {
				FullLogDetails.Analysis_details.SpecificProcess[proc] = strings.Join(proc_content, "\n")
			}
			waitGroup.Done()
		}(proc, proc_rgx)
	}
	waitGroup.Wait()

}
func getIssueDetails(cfgFile *Config, fContent string, headerMap map[string]bool) {
	FullLogDetails.Analysis_details.Issues = make(map[string]map[string]string)
	specific_proc_content := make(map[string]string)
	var wg sync.WaitGroup
	wg.Add(len(cfgFile.Issues))

	for issue_name, issue := range cfgFile.Issues {
		go func(issue_name string, issue Issue) {
			FullLogDetails.Analysis_details.Issues[issue_name] = make(map[string]string)
			//Filter the logs belonging to the issue specific process
			issueContent := ""
			for proc, proc_rgx := range issue.specific_process {
				proc_issue, ok := FullLogDetails.Analysis_details.SpecificProcess[proc]
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
				groupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name)

			} else {
				nongroupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name)
			}
			wg.Done()
		}(issue_name, issue)
	}
	wg.Wait()
}
func groupIssueDetails(issue Issue, cfgFile *Config, headerMap map[string]bool, issueContent string, issue_name string) {
	group_rgx, err := regexp.Compile(issue.grouping)
	if err != nil {
		return
	}
	group_names := group_rgx.SubexpNames()
	group_content := make(map[string][][]string)
	group_count := make(map[string][]int)
	last_matches, issues_count := fillGroupDetails(group_content, group_count, issueContent, group_rgx)
	issue_map := FullLogDetails.Analysis_details.Issues[issue_name]
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
		setFieldContent(field, field_rgx, issue_name, issueContent)
	}

	for field, field_rgx := range issue.additional_fields {
		setFieldContent(field, field_rgx, issue_name, issueContent)
		headerMap[field] = true
	}

	groupedDetails := GroupedStruct{}
	groupedDetails.Group_content = group_content
	groupedDetails.Group_count = group_count
	groupedDetails.Group_names = group_names
	FullLogDetails.GroupedIssues[issue_name] = groupedDetails
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
func nongroupIssueDetails(issue Issue, cfgFile *Config, headerMap map[string]bool, issueContent string, issue_name string) {
	issue_rgx, err := regexp.Compile(issue.regex)
	if err != nil {
		return
	}
	filter_logs := issue_rgx.FindAllString(issueContent, -1)
	filter_logs_map := make(map[string]bool)
	for _, filter_log := range filter_logs {
		filter_logs_map[filter_log] = true
	}
	FullLogDetails.NonGroupedIssues[issue_name] = filter_logs_map
	issue_map := FullLogDetails.Analysis_details.Issues[issue_name]
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
			setFieldContent(field, field_rgx, issue_name, issueContent)
		}
		for field, field_rgx := range issue.additional_fields {
			setFieldContent(field, field_rgx, issue_name, issueContent)
			headerMap[field] = true
		}
		timestampRegex, _ := regexp.Compile(cfgFile.IssuesGeneralFields.Timestamp)
		match = timestampRegex.FindStringSubmatch(filter_logs[len(filter_logs)-1])
		if len(match) > 0 {
			issue_map["Timestamp"] = match[0]
		}
	}
}
func setFieldContent(field string, field_rgx string, issue_name string, issueContent string) {
	field_rgx_comp, err := regexp.Compile(field_rgx)
	if err != nil {
		return
	}
	match := field_rgx_comp.FindAllString(issueContent, -1)
	fieldContent := strconv.Itoa(len(match)) + " :  " + strings.Join(match, "\n")
	FullLogDetails.Analysis_details.Issues[issue_name][field] = fieldContent
}
