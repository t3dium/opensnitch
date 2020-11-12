package firewall

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gustavo-iniguez-goya/opensnitch/daemon/log"
)

var (
	configFile    = "/etc/opensnitchd/system-fw.json"
	configWatcher *fsnotify.Watcher
	fwConfig      config
)

type fwRule struct {
	Description      string
	Table            string
	Chain            string
	Parameters       string
	Target           string
	TargetParameters string
}

type rulesList struct {
	Rule *fwRule
}

type config struct {
	sync.RWMutex
	SystemRules []*rulesList
}

func loadDiskConfiguration(reload bool) {
	raw, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Errorf("Error loading disk firewall configuration %s: %s", configFile, err)
	}

	if ok := loadConfiguration(raw); ok {
		if err := configWatcher.Add(configFile); err != nil {
			log.Error("Could not watch firewall configuration: %s", err)
			return
		}
	}

	if reload {
		return
	}

	go monitorConfigWorker()
}

// loadConfigutation reads the system firewall rules from disk.
// Then the rules are added based on the configuration defined.
func loadConfiguration(rawConfig []byte) bool {
	fwConfig.Lock()
	defer fwConfig.Unlock()

	DeleteSystemRules(true)
	if rulesChecker != nil {
		rulesChecker.Stop()
	}

	if err := json.Unmarshal(rawConfig, &fwConfig); err != nil {
		log.Error("Error parsing firewall configuration %s: %s", configFile, err)
		return false
	}

	for _, r := range fwConfig.SystemRules {
		if r.Rule.Chain == "" {
			continue
		}
		CreateSystemRule(r.Rule, true)
		AddSystemRule(ADD, r.Rule, true)
	}

	rulesChecker = time.NewTicker(time.Second * 30)
	return true
}

func saveConfiguration(rawConfig string) error {
	conf, err := json.Marshal([]byte(rawConfig))
	if err != nil {
		log.Error("saving json firewall configuration: ", err, conf)
		return err
	}

	if loadConfiguration([]byte(rawConfig)) != true {
		return fmt.Errorf("Error parsing firewall configuration %s: %s", rawConfig, err)
	}

	if err = ioutil.WriteFile(configFile, []byte(rawConfig), 0644); err != nil {
		log.Error("writing firewall configuration to disk: ", err)
		return err
	}
	return nil
}

func monitorConfigWorker() {
	for {
		select {
		case <-rulesCheckerChan:
			return
		case event := <-configWatcher.Events:
			if (event.Op&fsnotify.Write == fsnotify.Write) || (event.Op&fsnotify.Remove == fsnotify.Remove) {
				loadDiskConfiguration(true)
			}
		}
	}
}
