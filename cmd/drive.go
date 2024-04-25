package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	ucli "github.com/urfave/cli/v2"

	"github.com/seborama/pcloud-drive/v1/fuse"
	"github.com/seborama/pcloud/sdk"
)

func drive(c *ucli.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sdkHTTPClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   2,
			MaxConnsPerHost:       10,
			ResponseHeaderTimeout: 20 * time.Second,
			Proxy:                 http.ProxyFromEnvironment,
		},
		Timeout: 0,
	}

	pCloudClient := sdk.NewClient(sdkHTTPClient)

	slog.Info("logging into pCloud")
	err := pCloudClient.Login(
		ctx,
		c.String("pcloud-otp-code"),
		sdk.WithGlobalOptionUsername(c.String("pcloud-username")),
		sdk.WithGlobalOptionPassword(c.String("pcloud-password")),
	)
	if err != nil {
		return err
	}

	slog.Info("creating drive")
	drive, err := fuse.NewDrive(
		c.String("mount-point"),
		pCloudClient,
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = drive.Unmount() }()

	slog.Info("mouting FS", "location", c.String("mount-point"))
	err = drive.Mount()
	if err != nil {
		panic(err)
	}

	return nil
}
