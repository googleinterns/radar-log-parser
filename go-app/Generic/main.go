package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type config struct {
	SpecificProcess     map[string]string
	IssuesGeneralFields struct {
		Number      string
		Details     string
		Timestamp   string
		Log_level   string
		OtherFields map[string]string
	}
	Issues   map[string]issue
	Priority map[string]int
}

type configInterface struct {
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

type issue struct {
	specific_process  map[string]string
	regex             string
	detailing_mode    string
	grouping          string
	additional_fields map[string]string
}

type analysisDetails struct {
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
	analysis_details analysisDetails = analysisDetails{}
	GroupedIssues                    = make(map[string]GroupedStruct)
	NonGroupedIssues                 = make(map[string]map[string]bool)
)

var (
	homeTempl   = template.Must(template.ParseFiles("home.html"))
	reportTempl = template.Must(template.ParseFiles("report.html"))
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("assets"))
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))

	mux.HandleFunc("/report/", logReport)
	mux.HandleFunc("/", homeHandler)

	http.ListenAndServe(":"+port, mux)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		homeTempl.Execute(w, nil)
		return
	}
	//fScanner, fName, err := uploadFile(w, r)
	_, fName, err := uploadFile(w, r)
	if err != nil {
		return
	}
	cfgFile, err := extractConfig("./config.yml")
	if err != nil {
		return
	}
	analysis_details.FileName = *fName
	//fContent := extractFileContent(fScanner)
	fContent := extractFileContent() //test
	analysis_details.RawLog = fContent

	//Get the SpecificProcess logs
	analysis_details.SpecificProcess = make(map[string]string)
	for proc, proc_rgx := range cfgFile.SpecificProcess {
		proc_rgx_comp, err := regexp.Compile(proc_rgx)
		if err != nil {
			continue
		}
		analysis_details.SpecificProcess[proc] = strings.Join(proc_rgx_comp.FindAllString(fContent, -1), "\n")
	}
	//Fill the header with general fields
	headerMap := map[string]bool{"Issue": true, "Number": true, "Details": true, "Timestamp": true, "LogLevel": true}
	for field, _ := range cfgFile.IssuesGeneralFields.OtherFields {
		headerMap[field] = true
	}
	//Get the issues analysis details
	analysis_details.Issues = make(map[string]map[string]string)
	specific_proc_content := make(map[string]string)
	for issue_name, issue := range cfgFile.Issues {
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
			group_rgx, err := regexp.Compile(issue.grouping)
			if err != nil {
				continue
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
			analysis_details.Issues[issue_name]["Timestamp"] = timestampRegex.FindStringSubmatch(last_matches)[0]

			log_rgx, err := regexp.Compile(cfgFile.IssuesGeneralFields.Log_level)
			if err != nil {
				continue
			}
			match := log_rgx.FindStringSubmatch(last_matches)
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

		} else {
			issue_rgx, err := regexp.Compile(issue.regex)
			if err != nil {
				continue
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
					continue
				}
				match := log_rgx.FindStringSubmatch(filter_logs[0])
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
				timestampRegex, _ := regexp.Compile(cfgFile.IssuesGeneralFields.Timestamp)
				analysis_details.Issues[issue_name]["Timestamp"] = timestampRegex.FindStringSubmatch(filter_logs[len(filter_logs)-1])[0]

			}

		}
	}

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
	reportTempl.Execute(w, analysis_details)
}

func extractFileContent() string {
	data, err := ioutil.ReadFile("/usr/local/google/home/bancelidorcas/Documents/dev.txt")
	if err != nil {
		fmt.Println("File reading error", err)
		return ""
	}
	return string(data)
}

/*func extractFileContent(scanner *bufio.Scanner) string {
	var fContent string
	for scanner.Scan() {
		fContent += scanner.Text()
		fContent += "\n"
	}
	return fContent
}*/

func extractConfig(cfgPath string) (*config, error) {
	file, err := os.Open(cfgPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decodedFile := yaml.NewDecoder(file)
	cfg := &configInterface{}
	err = decodedFile.Decode(&cfg)
	if err != nil {
		return nil, err
	}
	cfgFile := config{}
	cfgFile.IssuesGeneralFields.Details = cfg.IssuesGeneralFields.Details
	cfgFile.IssuesGeneralFields.Log_level = cfg.IssuesGeneralFields.Log_level
	cfgFile.IssuesGeneralFields.Number = cfg.IssuesGeneralFields.Number
	cfgFile.IssuesGeneralFields.OtherFields = cfg.IssuesGeneralFields.OtherFields
	cfgFile.IssuesGeneralFields.Timestamp = cfg.IssuesGeneralFields.Timestamp
	cfgFile.Priority = cfg.Priority
	cfgFile.SpecificProcess = cfg.SpecificProcess
	cfgFile.Issues = make(map[string]issue)
	for issue_name, _ := range cfg.Issues {
		myIssues := issue{}
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

func uploadFile(w http.ResponseWriter, r *http.Request) (*bufio.Scanner, *string, error) {
	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("myFile")
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, nil, err
	}
	defer gz.Close()
	return bufio.NewScanner(gz), &handler.Filename, nil
}

func logReport(w http.ResponseWriter, r *http.Request) {
	switch file := r.URL.Path[len("/report/"):]; file {
	case analysis_details.FileName:
		template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
			return 0
		}, "addLine": func(x, y string) string {
			return ""
		}, "hightlightIssue": func(line string) bool {
			return true
		}, "detailType": func() string { return "Log" }}).ParseFiles("details.html")).Execute(w, analysis_details.RawLog)
	default:
		if file[:7] == "Details" {
			issue_name := r.URL.Path[len("/report/Details/"):]
			details, ok := GroupedIssues[issue_name]
			if ok {
				template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
					return 0
				}, "addLine": func(x, y string) string {
					return ""
				}, "hightlightIssue": func(line string) bool {

					return true
				}, "detailType": func() string { return "Group" }}).ParseFiles("details.html")).Execute(w, details)
			} else {
				template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
					return x + y
				}, "addLine": func(x, y string) string {
					return x + "\n" + y
				}, "hightlightIssue": func(line string) bool {
					_, ok := NonGroupedIssues[issue_name][line]
					return ok
				}, "detailType": func() string { return "nonGroup" }}).ParseFiles("details.html")).Execute(w, strings.Split(analysis_details.RawLog, "\n"))
			}
		} else {
			template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
				return 0
			}, "addLine": func(x, y string) string {
				return ""
			}, "hightlightIssue": func(line string) bool {

				return true
			}, "detailType": func() string { return "Log" }}).ParseFiles("details.html")).Execute(w, analysis_details.SpecificProcess[file])

		}
	}

}
