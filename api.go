package rm

import (
	"errors"
	"fmt"
	"path"
	"strings"

	rmApi "github.com/juruen/rmapi/api"
	rmLog "github.com/juruen/rmapi/log"
	rmModel "github.com/juruen/rmapi/model"
	rmTransport "github.com/juruen/rmapi/transport"
	rmUtil "github.com/juruen/rmapi/util"
)

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")
var ErrApi = errors.New("cloud API call error")

type Connection struct {
	apiCtx *rmApi.ApiCtx
}

type Auth struct {
	auth *rmModel.AuthTokens
}

func NewConnection(deviceToken, userToken string) (*Connection, error) {
	rmLog.InitLog() // TODO fix upstream
	if len(deviceToken) <= 0 {
		return nil, errors.New("empty reMarkable device token")
	}
	if len(userToken) <= 0 {
		return nil, errors.New("empty reMarkable user token")
	}
	auth := rmModel.AuthTokens{DeviceToken: deviceToken, UserToken: userToken}
	transport := rmTransport.CreateHttpClientCtx(auth)
	apiCtx, err := rmApi.CreateApiCtx(&transport)
	return &Connection{apiCtx}, err
}

func NewUserToken(deviceToken string) (string, error) {
	auth := rmModel.AuthTokens{DeviceToken: deviceToken, UserToken: ""}
	conn := rmTransport.CreateHttpClientCtx(auth)
	resp := rmTransport.BodyString{}
	err := conn.Post(rmTransport.DeviceBearer, "https://my.remarkable.com/token/json/2/user/new", nil, &resp)
	if err != nil {
		return "", fmt.Errorf("failed to create a new reMarkable user token: %w", err)
	}
	return resp.Content, nil
}

func (s *Connection) MkDir(target string) error {
	target = strings.Trim(strings.TrimSpace(target), "/")
	// Check if directory already exists
	node, err := s.apiCtx.Filetree.NodeByPath(target, s.apiCtx.Filetree.Root())
	if err == nil {
		if !node.IsDirectory() {
			return fmt.Errorf("destination path %s: %w", target, ErrAlreadyExists)
		}
		return nil
	}
	// Ensure that target's parent directory exists
	parentDir := path.Dir(target)
	newDir := path.Base(target)
	parentNode, err := s.apiCtx.Filetree.NodeByPath(parentDir, s.apiCtx.Filetree.Root())
	if err != nil || parentNode.IsFile() {
		return fmt.Errorf("parent directory %s: %w", parentDir, ErrNotFound)
	}
	// Create directory from parent node
	parentID := parentNode.Id()
	if parentNode.IsRoot() {
		parentID = ""
	}
	document, err := s.apiCtx.CreateDir(parentID, newDir)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %s: %w", newDir, err, ErrApi)
	}
	s.apiCtx.Filetree.AddDocument(document)
	return nil
}

func (s *Connection) Put(srcName, destDir string) error {
	destDir = strings.Trim(strings.TrimSpace(destDir), "/")
	docName, _ := rmUtil.DocPathToName(srcName)

	destNode, err := s.apiCtx.Filetree.NodeByPath(destDir, s.apiCtx.Filetree.Root())
	if err != nil || destNode.IsFile() {
		return fmt.Errorf("destination directory %s: %w", destDir, ErrNotFound)
	}

	_, err = s.apiCtx.Filetree.NodeByPath(docName, destNode)
	if err == nil {
		return fmt.Errorf("destination file %s: %w", docName, ErrAlreadyExists)
	}

	document, err := s.apiCtx.UploadDocument(destNode.Id(), srcName)
	if err != nil {
		return fmt.Errorf("failed to upload file %s: %s: %w", srcName, err, ErrApi)
	}
	s.apiCtx.Filetree.AddDocument(*document)
	return nil
}
