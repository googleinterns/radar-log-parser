package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/PuerkitoBio/goquery"
	"google.golang.org/api/iterator"
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

type CloudConfigDetails struct {
	Content              string
	Cloud_configurations map[string][]string
}

var (
	analysis_details analysisDetails = analysisDetails{}
	GroupedIssues                    = make(map[string]GroupedStruct)
	NonGroupedIssues                 = make(map[string]map[string]bool)
)

var (
	homeTempl          = template.Must(template.ParseFiles("home.html"))
	reportTempl        = template.Must(template.ParseFiles("report.html"))
	upload_configTempl = template.Must(template.ParseFiles("upload_config_home.html"))
	edit_configTempl   = template.Must(template.ParseFiles("editConfig.html"))
	delete_configTempl = template.Must(template.ParseFiles("deleteConfig.html"))
)
var (
	project_id           string   = "log-parser-278319"
	region_id            string   = "ue"
	app_specific_buckets []string = []string{"log-parser-278319.appspot.com", "staging.log-parser-278319.appspot.com", "us.artifacts.log-parser-278319.appspot.com"}
) //totoo put in a config file later
var cloudConfigs map[string][]string = make(map[string][]string)

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
	buckets, err := getBuckets()
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
			cfg, err := getConfigFiles(bucket)
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
				configs := CloudConfigDetails{
					Cloud_configurations: cloudConfigs,
					Content:              "",
				}
				edit_configTempl.Execute(w, configs)
			} else if strings.Contains(page, "deleteConfig") {
				delete_configTempl.Execute(w, cloudConfigs)
			} else {
				logReport(w, r)
			}
		} else {
			homeTempl.Execute(w, cloudConfigs)
		}
		return
	}
	switch page {
	case "UploadConfig":
		err := uploadConfigFile(r)
		if err != nil {
			return
		}
	case "editConfig":
		action := r.FormValue("action")
		if action == "Display" {
			configs, err := displayConfig(w, r)
			if err != nil {
				return
			}
			edit_configTempl.Execute(w, configs)
		} else {
			err := editConfig(w, r)
			if err != nil {
				return
			}
			homeTempl.Execute(w, cloudConfigs)
		}
	case "deleteConfig":
		err := deleteConfig(w, r)
		if err != nil {
			return
		}
		delete_configTempl.Execute(w, cloudConfigs)
	default:
		analyseLog(w, r)

	}

}
func deleteConfig(w http.ResponseWriter, r *http.Request) error {
	r.ParseMultipartForm(10 << 20)
	res, err := http.Get("https://" + project_id + "." + region_id + "." + "r.appspot.com" + r.URL.Path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return err
	}
	selectedBucket, found := doc.Find("optgroup").Attr("label")
	if !found {
		return err
	}
	cfgfile := r.FormValue("selectedFile")
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	o := client.Bucket(selectedBucket).Object(cfgfile)
	err = o.Delete(ctx)
	if err != nil {
		return fmt.Errorf("Object(%q).Delete: %v", cfgfile, err)
	}
	//update cloud config
	for i, _ := range cloudConfigs[selectedBucket] {
		if cloudConfigs[selectedBucket][i] == cfgfile {
			cloudConfigs[selectedBucket][i] = cloudConfigs[selectedBucket][len(cloudConfigs[selectedBucket])-1]
			cloudConfigs[selectedBucket] = cloudConfigs[selectedBucket][:len(cloudConfigs[selectedBucket])-1]
		}
	}
	return nil
}
func displayConfig(w http.ResponseWriter, r *http.Request) (*CloudConfigDetails, error) {
	r.ParseMultipartForm(10 << 20)
	cfgfile := r.FormValue("selectedFile")
	res, err := http.Get("https://" + project_id + "." + region_id + "." + "r.appspot.com/" + r.URL.Path)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}
	selector := "option[value=" + cfgfile + "]"
	selectedBucket, found := doc.Find(selector).Parent().Attr("label")
	if !found {
		return nil, err
	}
	content, err := downloadFile(w, selectedBucket, cfgfile)
	if err != nil {
		//http.Error(w, "Sorry, something went wrong", http.StatusInternalServerError)
		return nil, err
	}
	configs := CloudConfigDetails{
		Cloud_configurations: cloudConfigs,
		Content:              string(content),
	}
	return &configs, err

}
func editConfig(w http.ResponseWriter, r *http.Request) error {
	r.ParseMultipartForm(10 << 20)
	cfgfile := r.FormValue("selectedFile")
	res, err := http.Get("https://" + project_id + "." + region_id + "." + "r.appspot.com/" + r.URL.Path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return err
	}
	selectedBucket, found := doc.Find("optgroup").Attr("label")
	if !found {
		return err
	}

	//delete current file before
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	o := client.Bucket(selectedBucket).Object(cfgfile)
	err = o.Delete(ctx)
	if err != nil {
		return fmt.Errorf("Object(%q).Delete: %v", cfgfile, err)
	}
	//Replace with new content
	newContent := r.FormValue("configContent")
	wc := client.Bucket(selectedBucket).Object(cfgfile).NewWriter(ctx)
	_, err = io.WriteString(wc, newContent)
	if err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return nil
}

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
			match := timestampRegex.FindStringSubmatch(last_matches)
			if len(match) > 0 {
				analysis_details.Issues[issue_name]["Timestamp"] = match[0]
			}

			log_rgx, err := regexp.Compile(cfgFile.IssuesGeneralFields.Log_level)
			if err != nil {
				continue
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
				match = timestampRegex.FindStringSubmatch(filter_logs[len(filter_logs)-1])
				if len(match) > 0 {
					analysis_details.Issues[issue_name]["Timestamp"] = match[0]
				}

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

func extractConfig(cfgName string, bucket string) (*config, error) {
	cfg_data, err := downloadFile(nil, bucket, cfgName)
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

func uploadConfigFile(r *http.Request) error {
	r.ParseMultipartForm(10 << 20)
	selectedBucket := r.FormValue("selectedFile")
	file, handler, err := r.FormFile("myFile")
	if err != nil {
		return err
	}
	if filepath.Ext(handler.Filename) != ".yml" && filepath.Ext(handler.Filename) != ".yaml" {
		return errors.New("Invalid Format")
	}
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil
	}

	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()
	wc := client.Bucket(selectedBucket).Object(handler.Filename).NewWriter(ctx)
	//wc := client.Bucket(bucket).Object(handler.Filename).NewWriter(ctx)
	/*if _, err = io.WriteString(wc, "Bonjour par la"); err != nil {//will be used for update
		return false, err
	}*/
	if _, err = io.Copy(wc, file); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	//update config file
	cloudConfigs[selectedBucket] = append(cloudConfigs[selectedBucket], handler.Filename)
	return nil
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
	if filepath.Ext(handler.Filename) != ".gz" && filepath.Ext(handler.Filename) != ".txt" {
		return "", nil, cfg_file, selectedBucket, errors.New("Invalid Format")
	}
	defer file.Close()
	if filepath.Ext(handler.Filename) == ".gz" {
		gz, err := gzip.NewReader(file)
		if err != nil {
			return "", nil, cfg_file, selectedBucket, err
		}
		defer gz.Close()
		fContent := ""
		scanner := bufio.NewScanner(gz)
		for scanner.Scan() {
			fContent += scanner.Text()
			fContent += "\n"
		}
		return fContent, &handler.Filename, cfg_file, selectedBucket, nil
	} else {
		if err != nil {
			return "", nil, cfg_file, selectedBucket, err
		}
		data, err := ioutil.ReadAll(file)
		if err != nil {
			return "", nil, cfg_file, selectedBucket, err
		}
		return string(data), &handler.Filename, cfg_file, selectedBucket, nil
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

}

func downloadFile(w io.Writer, bucket, object string) ([]byte, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()
	rc, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("Object(%q).NewReader: %v", object, err)
	}

	defer rc.Close()

	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll: %v", err)
	}
	return data, nil
}
func getBuckets() ([]string, error) {
	ctx := context.Background()
	var buckets []string
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	it := client.Buckets(ctx, project_id)
	for {
		battrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, battrs.Name)
	}
	return buckets, nil
}

func getConfigFiles(bucket string) ([]string, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	it := client.Bucket(bucket).Objects(ctx, nil)
	var configs []string
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		configs = append(configs, attrs.Name)
	}
	return configs, nil
}
