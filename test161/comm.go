package main

import (
	"encoding/json"
	"github.com/parnurzeal/gorequest"
	"net/http"
)

type PostEndpoint string

const (
	ApiEndpointSubmit   PostEndpoint = "/api-v1/submit"
	ApiEndpointValidate              = "/api-v1/validate"
	ApiEndpointUpload                = "/api-v1/upload"
)

type SupportedPostType string

const (
	PostTypeJSON      SupportedPostType = "json"
	PostTypeMultipart                   = "multipart"
	PostTypeText                        = "text"
)

type PostRequest struct {
	Endpoint string
	SA       *gorequest.SuperAgent
}

func NewPostRequest(endpoint PostEndpoint) *PostRequest {
	ep := clientConf.Server + string(endpoint)

	return &PostRequest{
		Endpoint: ep,
		SA:       gorequest.New().Post(ep),
	}
}

func (pr *PostRequest) SetType(t SupportedPostType) {
	pr.SA.Type(string(t))
}

func (pr *PostRequest) QueueJSON(obj interface{}, fieldname string) error {
	if bytes, err := json.Marshal(obj); err != nil {
		return err
	} else {
		// A limitiation here is that we can't send complicated JSON objects.
		// For example, sending an obj with a list as a field truncates things.
		// Instead, we'll send a single field (a=b) where (a) is the expected
		// field name, and (b) is a JSON string.
		//
		// For compatibility sake, if fieldname is empty, we'll just send the
		// JSON  string.
		if len(fieldname) > 0 {
			pr.SA.Send(fieldname + "=" + string(bytes))
		} else {
			pr.SA.Send(string(bytes))
		}
		return nil
	}
}

func (pr *PostRequest) QueueFile(fileOnDisk, fileFieldName string) {
	pr.SA.SendFile(fileOnDisk, "", fileFieldName)
}

func (pr *PostRequest) Submit() (*http.Response, string, []error) {
	return pr.SA.End()
}
