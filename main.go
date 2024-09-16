package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"sync"
)

// Peer represents a Tailscale device in the same tailnet as the
// device from which this command is run.
type Peer struct {
	Hostname  string   `json:"Hostname"`
	Addresses []string `json:"TailscaleIPs"`
	Online    bool     `json:"Online"`
	Tags      []string `json:"Tags"`
}

// Response struct
type Response struct {
	Peers map[string]Peer `json:"Peer"`
}

// Parse the tailscale status for peer devices
func getDevices() ([]Peer, error) {
	// Users the CLI instead of the API for a few of reasons:
	// 1. The API doesn't support the "online" field.
	// 2. We really only want to ssh to Peers and not to Self.
	// 3. In small tailnets we may not even need to filter by tag.
	cmd := exec.Command("tailscale", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Now, try to unmarshal the output
	var r Response
	if err := json.Unmarshal(output, &r); err != nil {
		return nil, err
	}

	// Convert the map of Peers to a slice
	var peers []Peer
	for _, peer := range r.Peers {
		peers = append(peers, peer)
	}

	return peers, nil
}

// Check if the device has the specified tag
func hasTag(device Peer, tag string) bool {
	for _, t := range device.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Run SSH command on a device
func runSSHCommand(device Peer, sshUser, sshCommand string, wg *sync.WaitGroup) {
	defer wg.Done()
	if len(device.Addresses) == 0 {
		log.Printf("Device %s has no IP addresses.\n", device.Hostname)
		return
	}
	ip := device.Addresses[0] // Use the first IP address
	log.Printf("Running ssh command on device %s (%s)\n", device.Hostname, ip)

	// Prepare SSH command
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", fmt.Sprintf("%s@%s", sshUser, ip), sshCommand)

	// Run the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("The ssh command failed on device %s: %v\n", device.Hostname, err)
		return
	}

	// Output the result
	log.Printf("%s ssh command output:\n%s\n", device.Hostname, string(output))
}

func main() {
	// Command-line flags
	sshUser := flag.String("sshuser", "root", "SSH user")
	sshCommand := flag.String("sshcommand", "echo Hello from $HOST", "SSH command to run")
	deviceTag := flag.String("tag", "", "Filter devices by tag (e.g., tag:example)")
	flag.Parse()

	// Get the list of peer devices from Tailscale
	devices, err := getDevices()
	if err != nil {
		log.Fatalf("Error getting devices: %v\n", err)
	}

	// WaitGroup to manage goroutines
	var wg sync.WaitGroup

	// Iterate over devices and update online ones with the specified tag
	for _, device := range devices {
		if *deviceTag != "" && !hasTag(device, *deviceTag) {
			continue
		}
		if !device.Online {
			continue
		}
		wg.Add(1)
		go runSSHCommand(device, *sshUser, *sshCommand, &wg)
	}

	// Wait for all ssh commands to finish
	wg.Wait()
	log.Println("All ssh commands completed.")
}
