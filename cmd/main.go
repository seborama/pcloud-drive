package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "pcloud-username",
				EnvVars:  []string{"PCLOUD_USERNAME"},
				Usage:    "pCloud account username",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "pcloud-password",
				EnvVars:  []string{"PCLOUD_PASSWORD"},
				Usage:    "pCloud account password",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "pcloud-otp-code",
				EnvVars: []string{"PCLOUD_OTP_CODE"},
				Usage:   "pCloud account login One-Time-Password (for two-factor authentication)",
			},
		},

		Commands: []*cli.Command{
			{
				Name:    "drive",
				Aliases: []string{"d"},
				Usage:   "pCloud FUSE drive",
				Action:  drive,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "mount-point",
						Usage:    "Location of mount point",
						Required: true,
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("%+v", err)
	}
}
