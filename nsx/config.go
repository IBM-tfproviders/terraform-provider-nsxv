package nsx

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/IBM-tfproviders/govnsx"
)

type Config struct {
	User          string
	Password      string
	NsxManagerUri string
	InsecureFlag  bool
	Debug         bool
	DebugPath     string
	DebugPathRun  string
}

//
// Client() returns a new client for accessing NSX manager
//
func (c *Config) Client() (*govnsx.Client, error) {

	err := c.EnableDebug()
	if err != nil {
		return nil, fmt.Errorf("Error setting up client debug: %s", err)
	}

	nsxMgrParams := &govnsx.NsxManagerConfig{
		UserName:      c.User,
		Password:      c.Password,
		Uri:           c.NsxManagerUri,
		AllowInsecssl: c.InsecureFlag,
		UserAgentName: "govnsx",
	}

	client, err := govnsx.NewClient(nsxMgrParams)
	if err != nil {
		return nil, fmt.Errorf("Error setting up client: %s", err)
	}

	log.Printf("[INFO] NSX Manager Client configured for URL: %s",
		c.NsxManagerUri)

	return client, nil
}

func (c *Config) EnableDebug() error {
	if !c.Debug {
		return nil
	}

	// Base path for storing debug logs.
	r := c.DebugPath
	if r == "" {
		r = filepath.Join(os.Getenv("HOME"), ".govnsx")
	}
	r = filepath.Join(r, "debug")

	// Path for this particular run.
	run := c.DebugPathRun
	if run == "" {
		now := time.Now().Format("2006-01-02T15-04-05.999999999")
		r = filepath.Join(r, now)
	} else {
		// reuse the same path
		r = filepath.Join(r, run)
		_ = os.RemoveAll(r)
	}

	err := os.MkdirAll(r, 0700)
	if err != nil {
		log.Printf("[ERROR] Client debug setup failed: %v", err)
		return err
	}

	return nil
}
