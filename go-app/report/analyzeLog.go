package report

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"radar-log-parser/go-app/utilities"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v2"
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
	Issues   map[string]Issue
	Priority map[string]int
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
	Issues   map[string]interface{} `yaml:"Issues"`
	Priority map[string]int         `yaml:"Priority"`
}
type Issue struct {
	specific_process  map[string]string
	regex             string
	detailing_mode    string
	grouping          string
	additional_fields map[string]string
}
type AnalysisDetails struct {
	FileName        string
	RawLog          string
	SpecificProcess map[string]string
	Header          []string
	OrderedIssues   []string
	Issues          map[string]map[string]string
}

type GroupedStruct struct {
	Group_names   []string
	Group_content map[string][][]string
	Group_count   map[string][]int
}

var (
	analysis_details AnalysisDetails = AnalysisDetails{}
	GroupedIssues                    = make(map[string]GroupedStruct)
	NonGroupedIssues                 = make(map[string]map[string]bool)
)

func extractConfig(cfgName string, bucket string) (*Config, error) {
	cfg_data, err := utilities.DownloadFile(nil, bucket, cfgName)
	if err != nil {
		return nil, err
	}
	cfg := &ConfigInterface{}
	if err := yaml.Unmarshal(cfg_data, cfg); err != nil {
		return nil, err
	}
	cfgFile := Config{}
	cfgFile.IssuesGeneralFields.Details = cfg.IssuesGeneralFields.Details
	cfgFile.IssuesGeneralFields.Log_level = cfg.IssuesGeneralFields.Log_level
	cfgFile.IssuesGeneralFields.Number = cfg.IssuesGeneralFields.Number
	cfgFile.IssuesGeneralFields.OtherFields = cfg.IssuesGeneralFields.OtherFields
	cfgFile.IssuesGeneralFields.Timestamp = cfg.IssuesGeneralFields.Timestamp
	cfgFile.Priority = cfg.Priority
	cfgFile.SpecificProcess = cfg.SpecificProcess
	cfgFile.Issues = make(map[string]Issue)
	for issue_name, _ := range cfg.Issues {
		myIssues := Issue{}
		myIssues.specific_process = make(map[string]string)
		myIssues.additional_fields = make(map[string]string)
		for issue_key, issue_value := range cfg.Issues[issue_name].(map[interface{}]interface{}) {
			switch issue_value.(type) {
			case string:
				switch issue_key {
				case "regex":
					myIssues.regex = issue_value.(string)
				case "detailing_mode":
					myIssues.detailing_mode = issue_value.(string)
				case "grouping":
					myIssues.grouping = issue_value.(string)
				}

			case map[interface{}]interface{}:
				for name, value := range issue_value.(map[interface{}]interface{}) {
					if issue_key == "specific_process" {
						myIssues.specific_process[name.(string)] = value.(string)
					} else {
						myIssues.additional_fields[name.(string)] = value.(string)
					}
				}
			case interface{}:
			}
		}
		cfgFile.Issues[issue_name] = myIssues
	}
	return &cfgFile, nil
}
func uploadLogFile(w http.ResponseWriter, r *http.Request, project_id string, region_id string) (string, *string, string, string, error) {
	r.ParseMultipartForm(10 << 20)
	cfg_file := r.FormValue("selectedFile")
	res, err := http.Get("https://" + project_id + "." + region_id + "." + "r.appspot.com/" + r.URL.Path)
	if err != nil {
		return "", nil, cfg_file, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", nil, cfg_file, "", err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", nil, cfg_file, "", err
	}
	selectedBucket, found := doc.Find("optgroup").Attr("label")
	if !found {
		return "", nil, cfg_file, "", err
	}
	file, handler, err := r.FormFile("myFile")
	if err != nil {
		return "", nil, cfg_file, selectedBucket, err
	}
	content, err := extractLogContent(file, handler)
	if err != nil {
		return "", nil, cfg_file, selectedBucket, err
	}
	defer file.Close()
	return content, &handler.Filename, cfg_file, selectedBucket, nil
}
func extractLogContent(file multipart.File, handler *multipart.FileHeader) (string, error) {
	if filepath.Ext(handler.Filename) != ".gz" && filepath.Ext(handler.Filename) != ".txt" {
		return "", errors.New("Invalid Format")
	}
	if filepath.Ext(handler.Filename) == ".gz" {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		fContent := ""
		scanner := bufio.NewScanner(gz)
		for scanner.Scan() {
			fContent += scanner.Text()
			fContent += "\n"
		}
		return fContent, nil
	} else {
		data, err := ioutil.ReadAll(file)
		if err != nil {
			return "", nil
		}

		return string(data), nil
	}
}
func groupIssueDetails(issue Issue, cfgFile *Config, headerMap map[string]bool, issueContent string, issue_name string) {

	group_rgx, err := regexp.Compile(issue.grouping)
	if err != nil {
		return
	}
	group_names := group_rgx.SubexpNames()
	group_content := make(map[string][][]string)
	group_count := make(map[string][]int)
	last_matches := ""
	for _, log := range strings.Split(issueContent, "\n") {
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
	analysis_details.Issues[issue_name]["Number"] += strconv.Itoa(issues_count)

	timestampRegex, _ := regexp.Compile(cfgFile.IssuesGeneralFields.Timestamp)
	match := timestampRegex.FindStringSubmatch(last_matches)
	if len(match) > 0 {
		analysis_details.Issues[issue_name]["Timestamp"] = match[0]
	}

	log_rgx, err := regexp.Compile(cfgFile.IssuesGeneralFields.Log_level)
	if err != nil {
		return
	}
	match = log_rgx.FindStringSubmatch(last_matches)
	if len(match) > 1 {
		analysis_details.Issues[issue_name]["LogLevel"] = match[1]

	}
	for field, field_rgx := range cfgFile.IssuesGeneralFields.OtherFields {
		field_rgx_comp, err := regexp.Compile(field_rgx)
		if err != nil {
			continue
		}
		match := field_rgx_comp.FindAllString(issueContent, -1)
		analysis_details.Issues[issue_name][field] = strconv.Itoa(len(match)) + " :  " + strings.Join(match, "\n")

	}

	for field, field_rgx := range issue.additional_fields {
		field_rgx_comp, err := regexp.Compile(field_rgx)
		if err != nil {
			continue
		}
		match := field_rgx_comp.FindAllString(issueContent, -1)
		analysis_details.Issues[issue_name][field] = strconv.Itoa(len(match)) + " :  " + strings.Join(match, "\n")
		headerMap[field] = true
	}

	groupedDetails := GroupedStruct{}
	groupedDetails.Group_content = group_content
	groupedDetails.Group_count = group_count
	groupedDetails.Group_names = group_names
	GroupedIssues[issue_name] = groupedDetails
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
	NonGroupedIssues[issue_name] = filter_logs_map

	analysis_details.Issues[issue_name]["Number"] = strconv.Itoa(len(filter_logs))
	issueContent = strings.Join(filter_logs, "\n")

	if len(filter_logs) > 0 {
		log_rgx, err := regexp.Compile(cfgFile.IssuesGeneralFields.Log_level)
		if err != nil {
			return
		}
		match := log_rgx.FindStringSubmatch(filter_logs[0])
		if len(match) > 1 {
			analysis_details.Issues[issue_name]["LogLevel"] = match[1]
		}

		for field, field_rgx := range cfgFile.IssuesGeneralFields.OtherFields {
			field_rgx_comp, err := regexp.Compile(field_rgx)
			if err != nil {
				return
			}
			match := field_rgx_comp.FindAllString(issueContent, -1)
			analysis_details.Issues[issue_name][field] = strconv.Itoa(len(match)) + " :  " + strings.Join(match, "\n")

		}

		for field, field_rgx := range issue.additional_fields {
			field_rgx_comp, err := regexp.Compile(field_rgx)
			if err != nil {
				return
			}
			match := field_rgx_comp.FindAllString(issueContent, -1)
			analysis_details.Issues[issue_name][field] = strconv.Itoa(len(match)) + " :  " + strings.Join(match, "\n")
			headerMap[field] = true
		}

		timestampRegex, _ := regexp.Compile(cfgFile.IssuesGeneralFields.Timestamp)
		match = timestampRegex.FindStringSubmatch(filter_logs[len(filter_logs)-1])
		if len(match) > 0 {
			analysis_details.Issues[issue_name]["Timestamp"] = match[0]
		}
	}
}
func AnalyseLog(w http.ResponseWriter, r *http.Request, project_id string, region_id string) (AnalysisDetails, map[string]GroupedStruct, map[string]map[string]bool, error) {
	fScanner, fName, cfgName, bucket, err := uploadLogFile(w, r, project_id, region_id)
	if err != nil {
		return analysis_details, nil, nil, err
	}
	cfgFile, err := extractConfig(cfgName, bucket)
	if err != nil {
		return analysis_details, nil, nil, err
	}
	analysis_details.FileName = *fName
	fContent := fScanner
	analysis_details.RawLog = fContent

	//Get the SpecificProcess logs
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(cfgFile.SpecificProcess))
	analysis_details.SpecificProcess = make(map[string]string)
	for proc, proc_rgx := range cfgFile.SpecificProcess {
		go func(proc string, proc_rgx string) {
			proc_rgx_comp, err := regexp.Compile(proc_rgx)
			if err != nil {
				waitGroup.Done()
				return
			}
			proc_content := proc_rgx_comp.FindAllString(fContent, -1)
			if len(proc_content) > 1 {
				analysis_details.SpecificProcess[proc] = strings.Join(proc_rgx_comp.FindAllString(fContent, -1), "\n")
			}
			waitGroup.Done()
		}(proc, proc_rgx)
	}
	waitGroup.Wait()
	//Fill the header with general fields
	headerMap := map[string]bool{"Issue": true, "Number": true, "Details": true, "Timestamp": true, "LogLevel": true}
	for field, _ := range cfgFile.IssuesGeneralFields.OtherFields {
		headerMap[field] = true
	}
	//Get the issues analysis details
	analysis_details.Issues = make(map[string]map[string]string)
	specific_proc_content := make(map[string]string)
	var wg sync.WaitGroup
	wg.Add(len(cfgFile.Issues))

	for issue_name, issue := range cfgFile.Issues {
		go func(issue_name string, issue Issue) {
			analysis_details.Issues[issue_name] = make(map[string]string)
			//Filter the logs belonging to the issue specific process
			issueContent := ""
			for proc, proc_rgx := range issue.specific_process {
				proc_issue, ok := analysis_details.SpecificProcess[proc]
				if !ok {
					proc_issue, ok := specific_proc_content[proc]
					if !ok {
						proc_rgx_comp, err := regexp.Compile(proc_rgx)
						if err != nil {
							continue
						}
						proc_issue = strings.Join(proc_rgx_comp.FindAllString(fContent, -1), "\n")
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

	// Get all issues in a prioritized order
	issues := make([]string, 0, len(cfgFile.Issues))
	for k := range cfgFile.Issues {
		issues = append(issues, k)
	}
	sort.Slice(issues, func(i, j int) bool {
		return cfgFile.Priority[issues[i]] > cfgFile.Priority[issues[j]]
	})
	analysis_details.OrderedIssues = issues

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
	analysis_details.Header = header
	return analysis_details, GroupedIssues, NonGroupedIssues, nil
}
