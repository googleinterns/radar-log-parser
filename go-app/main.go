package main

import (
	"html/template"
	"net/http"
	"os"
	"radar-log-parser/go-app/report"
	"radar-log-parser/go-app/settings"
	"radar-log-parser/go-app/utilities"
	"strings"
)

/*type config struct {
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
}*/

var (
	analysis_details report.AnalysisDetails = report.AnalysisDetails{}
	GroupedIssues                           = make(map[string]report.GroupedStruct)
	NonGroupedIssues                        = make(map[string]map[string]bool)
)

var (
	homeTempl             = template.Must(template.ParseFiles("templates/home.html"))
	reportTempl           = template.Must(template.ParseFiles("templates/report.html"))
	upload_configTempl    = template.Must(template.ParseFiles("templates/upload_config_home.html"))
	edit_config_homeTempl = template.Must(template.ParseFiles("templates/editConfigHome.html"))
	edit_configTempl      = template.Must(template.ParseFiles("templates/editConfig.html"))

	delete_configTempl = template.Must(template.ParseFiles("templates/deleteConfig.html"))
)
var (
	project_id           string   = "log-parser-278319"
	region_id            string   = "ue"
	app_specific_buckets []string = []string{"log-parser-278319.appspot.com", "staging.log-parser-278319.appspot.com", "us.artifacts.log-parser-278319.appspot.com"}
) //totoo put in a config file later
var cloudConfigs map[string][]string = make(map[string][]string)
var (
	cfg_edit    string
	bucket_edit string
)

func main() {
	port := os.Getenv("PORT")
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("assets"))
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/", homeHandler)

	fillConfigMap()

	http.ListenAndServe(":"+port, mux)
}
func fillConfigMap() {

	buckets, err := utilities.GetBuckets(project_id)
	if err != nil {
		return
	}
	for _, bucket := range buckets {
		allow := true
		for _, buckt := range app_specific_buckets {
			if buckt == bucket {
				allow = false
				break
			}
		}
		if allow {
			cfg, err := utilities.GetConfigFiles(bucket)
			if err != nil {
				return
			}
			cloudConfigs[bucket] = cfg
		}

	}
}
func homeHandler(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Path[len("/"):]
	if r.Method != http.MethodPost {
		if len(page) > 5 {
			if strings.Contains(page, "UploadConfig") {
				bucketList := make([]string, 0, len(cloudConfigs))
				for k := range cloudConfigs {
					bucketList = append(bucketList, k)
				}
				upload_configTempl.Execute(w, bucketList)
			} else if strings.Contains(page, "analyzeLog") {
				homeTempl.Execute(w, cloudConfigs)
			} else if strings.Contains(page, "editConfig") {
				edit_config_homeTempl.Execute(w, cloudConfigs)
			} else if strings.Contains(page, "deleteConfig") {
				delete_configTempl.Execute(w, cloudConfigs)
			} else {
				report.LogReport(w, r, analysis_details, GroupedIssues, NonGroupedIssues)
				//logReport(w, r)
			}
		} else {
			homeTempl.Execute(w, cloudConfigs)
		}
		return
	}
	switch page {
	case "UploadConfig":
		configs, err := settings.UploadConfigFile(r, project_id, cloudConfigs)
		if err != nil {
			return
		}
		cloudConfigs = configs

		template.Must(template.ParseFiles("templates/feedback.html")).Execute(w, nil)
	case "editConfig":
		if r.FormValue("action") == "Save" {
			err := settings.SaveConfig(r, bucket_edit, cfg_edit)
			if err != nil {
				return
			}
			template.Must(template.ParseFiles("templates/feedback.html")).Execute(w, nil)
		} else {
			bck, cfg, content, err := settings.DisplayConfig(w, r, project_id, region_id)
			bucket_edit = bck
			cfg_edit = cfg
			if err != nil {
				return
			}
			edit_configTempl.Execute(w, content)
		}

	case "deleteConfig":
		configs, err := settings.DeleteConfig(r, project_id, region_id, cloudConfigs)
		if err != nil {
			return
		}
		cloudConfigs = configs
		template.Must(template.ParseFiles("templates/feedback.html")).Execute(w, nil)
		//delete_configTempl.Execute(w, cloudConfigs)
	default:
		details, grouped, non_grouped, err := report.AnalyseLog(w, r, project_id, region_id)
		if err != nil {
			return
		}
		analysis_details = details
		GroupedIssues = grouped
		NonGroupedIssues = non_grouped
		template.Must(template.ParseFiles("templates/report.html")).Execute(w, analysis_details)
		//analyseLog(w, r)

	}

}

/*
func analyseLog(w http.ResponseWriter, r *http.Request) {
	fScanner, fName, cfgName, bucket, err := uploadLogFile(w, r)
	if err != nil {
		return
	}
	cfgFile, err := extractConfig(cfgName, bucket)
	if err != nil {
		return
	}
	analysis_details.FileName = *fName
	fContent := fScanner
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
			groupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name)
		} else {
			nongroupIssueDetails(issue, cfgFile, headerMap, issueContent, issue_name)
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

func groupIssueDetails(issue issue, cfgFile *config, headerMap map[string]bool, issueContent string, issue_name string) {
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
func nongroupIssueDetails(issue issue, cfgFile *config, headerMap map[string]bool, issueContent string, issue_name string) {
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

func extractConfig(cfgName string, bucket string) (*config, error) {
	cfg_data, err := utilities.DownloadFile(nil, bucket, cfgName)
	if err != nil {
		return nil, err
	}
	cfg := &configInterface{}
	if err := yaml.Unmarshal(cfg_data, cfg); err != nil {
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

func uploadLogFile(w http.ResponseWriter, r *http.Request) (string, *string, string, string, error) {
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

func logReport(w http.ResponseWriter, r *http.Request) {
	switch file := r.URL.Path[len("/report/"):]; file {
	case analysis_details.FileName:
		template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
			return 0
		}, "addLine": func(x, y string) string {
			return x + "\n" + y
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
					return x + "\n" + y
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
				return x + "\n" + y
			}, "hightlightIssue": func(line string) bool {

				return true
			}, "detailType": func() string { return "Log" }}).ParseFiles("details.html")).Execute(w, analysis_details.SpecificProcess[file])

		}
	}

}*/
