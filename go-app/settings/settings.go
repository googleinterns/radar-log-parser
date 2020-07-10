package settings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"radar-log-parser/go-app/utilities"
	"time"

	"cloud.google.com/go/storage"
	"github.com/PuerkitoBio/goquery"
)

func UploadConfigFile(r *http.Request, project_id string, cloudConfigs map[string][]string) (map[string][]string, error) {
	r.ParseMultipartForm(10 << 20)
	selectedBucket := r.FormValue("selectedFile")
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return cloudConfigs, err
	}

	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()
	if selectedBucket == "Create Bucket" {
		selectedBucket = r.FormValue("bucketName")
		bucket := client.Bucket(selectedBucket)
		if err := bucket.Create(ctx, project_id, &storage.BucketAttrs{
			StorageClass: "STANDARD",
			Location:     "US",
		}); err != nil {
			return cloudConfigs, err
		}
	}
	file, handler, err := r.FormFile("myFile")
	if err != nil {
		return cloudConfigs, err
	}
	if filepath.Ext(handler.Filename) != ".yml" && filepath.Ext(handler.Filename) != ".yaml" {
		return cloudConfigs, errors.New("Invalid Format")
	}

	wc := client.Bucket(selectedBucket).Object(handler.Filename).NewWriter(ctx)
	if _, err = io.Copy(wc, file); err != nil {
		return cloudConfigs, err
	}
	if err := wc.Close(); err != nil {
		return cloudConfigs, err
	}
	//update config file
	cloudConfigs[selectedBucket] = append(cloudConfigs[selectedBucket], handler.Filename)

	return cloudConfigs, nil
}
func DeleteConfig(r *http.Request, project_id string, region_id string, cloudConfigs map[string][]string) (map[string][]string, error) {
	r.ParseMultipartForm(10 << 20)
	res, err := http.Get("https://" + project_id + "." + region_id + "." + "r.appspot.com" + r.URL.Path)
	if err != nil {
		return cloudConfigs, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return cloudConfigs, err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return cloudConfigs, err
	}
	selectedBucket, found := doc.Find("optgroup").Attr("label")
	if !found {
		return cloudConfigs, err
	}
	cfgfile := r.FormValue("selectedFile")
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return cloudConfigs, fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	o := client.Bucket(selectedBucket).Object(cfgfile)
	err = o.Delete(ctx)
	if err != nil {
		return cloudConfigs, fmt.Errorf("Object(%q).Delete: %v", cfgfile, err)
	}
	//update cloud config
	for i, _ := range cloudConfigs[selectedBucket] {
		if cloudConfigs[selectedBucket][i] == cfgfile {
			cloudConfigs[selectedBucket][i] = cloudConfigs[selectedBucket][len(cloudConfigs[selectedBucket])-1]
			cloudConfigs[selectedBucket] = cloudConfigs[selectedBucket][:len(cloudConfigs[selectedBucket])-1]
		}
	}
	return cloudConfigs, nil
}
func SaveConfig(r *http.Request, bucket_edit string, cfg_edit string) error {
	r.ParseMultipartForm(10 << 20)
	//delete current file before
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	o := client.Bucket(bucket_edit).Object(cfg_edit)
	err = o.Delete(ctx)
	if err != nil {
		return fmt.Errorf("Object(%q).Delete: %v", cfg_edit, err)
	}
	//Replace with new content
	newContent := r.FormValue("configContent")
	wc := client.Bucket(bucket_edit).Object(cfg_edit).NewWriter(ctx)
	_, err = io.WriteString(wc, newContent)
	if err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return nil
}

func DisplayConfig(w http.ResponseWriter, r *http.Request, project_id string, region_id string) (string, string, string, error) {
	r.ParseMultipartForm(10 << 20)
	cfgfile := r.FormValue("selectedFile")
	res, err := http.Get("https://" + project_id + "." + region_id + "." + "r.appspot.com/" + r.URL.Path)
	if err != nil {
		return "", "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", "", "", err
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", "", "", err
	}
	selectedBucket, found := doc.Find("optgroup").Attr("label")
	if !found {
		return "", "", "", err
	}
	content, err := utilities.DownloadFile(w, selectedBucket, cfgfile)
	if err != nil {
		return "", "", "", err
	}
	return selectedBucket, cfgfile, string(content), err

}
