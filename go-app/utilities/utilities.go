package utilities

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

func DownloadFile(w io.Writer, bucket, object string) ([]byte, error) {
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
func GetBuckets(project_id string) ([]string, error) {
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

func GetConfigFiles(bucket string) ([]string, error) {
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
