package rm

import (
	rmApi "github.com/juruen/rmapi/api"
	rmModel "github.com/juruen/rmapi/model"
	rmTransport "github.com/juruen/rmapi/transport"
)

type Connection struct {
	ctx *rmApi.ApiCtx
}

type Auth struct {
	auth *rmModel.AuthTokens
}

func NewConnection(deviceToken, userToken string) (*Connection, error) {
	auth := rmModel.AuthTokens{DeviceToken: deviceToken, UserToken: userToken}
	transport := rmTransport.CreateHttpClientCtx(auth)
	ctx, err := rmApi.CreateApiCtx(&transport)
	return &Connection{ctx}, err
}

func (s *Connection) MkDir(path string) error {
	_, err := s.ctx.CreateDir("", path)
	return err
}

func (s *Connection) Put(doc, dest string) error {
	_, err := s.ctx.UploadDocument(dest, doc)
	return err
}
