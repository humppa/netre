// netre, Copyright (c) 2017 Tuomas Starck

/* TODO
 * Create pre-script runner
 * Create post-script runner
 * Create actions for other systems
 * Publish public address if configured
 */

package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/miekg/dns"
	"github.com/nightlyone/lockfile"
	"github.com/spf13/viper"
)

type Check struct {
	Type     string
	Name     string
	Server   string
	Location string
}

const (
	programName     = "netre"
	configDirectory = "/etc/netre"
	lockFileName    = "netre.lock"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

func httpQuery(location string) (hasNet bool) {
	log.Printf("Requesting %q", location)

	hasNet = true

	client := http.Client{
		Timeout: time.Duration(viper.GetDuration("timeout")),
	}

	response, err := client.Head(location)

	if err != nil {
		log.Printf("%+v", err)
		hasNet = false
	}

	if response != nil {
		defer response.Body.Close()
	}

	return
}

func dnsQuery(server string, host string) (hasNet bool) {
	/*
	 * dial udp: missing address
	 * dns: domain must be fully qualified
	 * dial udp: missing port in address 8.8.8.8
	 * read udp 172.17.2.68:49765->8.8.8.1:53: i/o timeout
	 */
	log.Printf("Querying %q from %s", host, server)

	hasNet = true
	msg := dns.Msg{}
	client := dns.Client{}
	msg.SetQuestion(host, dns.TypeA)
	_, _, err := client.Exchange(&msg, server)

	if err != nil {
		log.Println("DNS query failed. Either there is no connectivity")
		log.Println("or check is invalid. Error message was:")
		log.Printf("%+v", err)
		hasNet = false
	}

	return
}

func checkForInternet() bool {
	var check Check
	checksMap := viper.GetStringMap("checks")

	for key := range checksMap {
		hasNet := true
		err := viper.UnmarshalKey("checks."+key, &check)

		if err != nil {
			log.Printf("Failed to parse a check from configuration\n%+v", err)
			return true
		}

		switch check.Type {
		case "dns":
			hasNet = dnsQuery(check.Server, check.Name)
		case "http":
			hasNet = httpQuery(check.Location)
		default:
			log.Printf("Unrecognized check type: %q", check.Type)
			log.Println("Cannot run connectivity check. Please fix configuration!")
		}

		if hasNet {
			return true
		}

		delay := viper.GetDuration("delay")
		log.Printf("Next check after %s", delay)
		time.Sleep(delay)
	}

	return false
}

// ifUpDown strategy should work at least for Ubuntu 14.04 to 16.04
func ifUpDown() {
	var out []byte
	out, _ = exec.Command("/bin/echo", "ifdown", "-a", "--exclude=lo").CombinedOutput()
	log.Printf("%s", out)
	out, _ = exec.Command("/bin/echo", "ifup", "-a", "--exclude=lo").CombinedOutput()
	log.Printf("%s", out)
}

func netre() int {
	if checkForInternet() {
		log.Print("Connection is good")
	} else {
		log.Print("Connection failed, restarting all interfaces")
		ifUpDown()
	}

	return 0
}

func acquireLock() int {
	lock, err := lockfile.New(filepath.Join(os.TempDir(), lockFileName))

	if err != nil {
		log.Printf("Unable to initialize the lock\n%+v", err)
		return 1
	}

	err = lock.TryLock()

	if err != nil {
		log.Printf("Failed to acquire lock %q", lock)
		log.Printf("Either another instance of netre is running")
		log.Printf("or lockfile needs to be removed manually.")
		return 0
	}

	defer lock.Unlock()

	return netre()
}

func main() {
	viper.SetConfigName(programName)
	viper.AddConfigPath(configDirectory)
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()

	if err != nil {
		log.Printf("Failed to read configuration\n%+v", err)
	}

	viper.SetDefault("delay", "1m")
	viper.SetDefault("timeout", "30s")

	os.Exit(acquireLock())
}
