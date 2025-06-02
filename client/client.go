package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	// Default chunk size for download test (1 MiB)
	defaultChunkSize = 1048576
	// Default number of chunks for download test
	defaultChunks = 4
	// Default timeout for HTTP requests
	defaultTimeout = 10 * time.Second
)

// Client represents a librespeed client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Result represents the speed test results
type Result struct {
	DownloadSpeed float64 // in Mbps
	UploadSpeed   float64 // in Mbps
	Ping          float64 // in ms
	Jitter        float64 // in ms
	ISP           string
	IP            string
}

// NewClient creates a new librespeed client
func NewClient(baseURL string) *Client {
	// Ensure baseURL doesn't end with a slash
	baseURL = strings.TrimRight(baseURL, "/")

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// RunTest performs a complete speed test
func (c *Client) RunTest() (*Result, error) {
	result := &Result{}

	// Get IP and ISP info first
	ipInfo, err := c.getIPInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get IP info: %w", err)
	}
	result.IP = ipInfo.ProcessedString
	result.ISP = ipInfo.RawISPInfo.Organization

	// Run ping test
	ping, jitter, err := c.pingTest()
	if err != nil {
		return nil, fmt.Errorf("ping test failed: %w", err)
	}
	result.Ping = ping
	result.Jitter = jitter

	// Run download test
	downloadSpeed, err := c.downloadTest()
	if err != nil {
		return nil, fmt.Errorf("download test failed: %w", err)
	}
	result.DownloadSpeed = downloadSpeed

	// Run upload test
	uploadSpeed, err := c.uploadTest()
	if err != nil {
		return nil, fmt.Errorf("upload test failed: %w", err)
	}
	result.UploadSpeed = uploadSpeed

	return result, nil
}

// downloadTest performs a download speed test
func (c *Client) downloadTest() (float64, error) {
	start := time.Now()

	// Request garbage data with default chunk size
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/garbage?ckSize=%d", c.baseURL, defaultChunks))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download test failed with status: %d", resp.StatusCode)
	}

	// Read all data
	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return 0, err
	}

	duration := time.Since(start).Seconds()
	// Calculate speed in Mbps
	// Total bytes = chunks * chunkSize
	totalBytes := float64(defaultChunks * defaultChunkSize)
	speedMbps := (totalBytes * 8) / (1000000 * duration) // Convert to Mbps

	return speedMbps, nil
}

// uploadTest performs an upload speed test
func (c *Client) uploadTest() (float64, error) {
	// Create a buffer with random data
	data := make([]byte, defaultChunkSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	start := time.Now()

	// Upload the data
	resp, err := c.httpClient.Post(fmt.Sprintf("%s/empty", c.baseURL), "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("upload test failed with status: %d", resp.StatusCode)
	}

	duration := time.Since(start).Seconds()
	// Calculate speed in Mbps
	speedMbps := (float64(len(data)) * 8) / (1000000 * duration)

	return speedMbps, nil
}

// pingTest performs a ping test
func (c *Client) pingTest() (float64, float64, error) {
	var pings []float64
	iterations := 5

	for i := 0; i < iterations; i++ {
		start := time.Now()

		resp, err := c.httpClient.Get(fmt.Sprintf("%s/empty", c.baseURL))
		if err != nil {
			return 0, 0, err
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return 0, 0, fmt.Errorf("ping test failed with status: %d", resp.StatusCode)
		}

		ping := float64(time.Since(start).Microseconds()) / 1000.0 // Convert to ms
		pings = append(pings, ping)

		// Small delay between pings
		time.Sleep(100 * time.Millisecond)
	}

	// Calculate average ping
	var sum float64
	for _, ping := range pings {
		sum += ping
	}
	avgPing := sum / float64(len(pings))

	// Calculate jitter (standard deviation of pings)
	var variance float64
	for _, ping := range pings {
		variance += math.Pow(ping-avgPing, 2)
	}
	jitter := math.Sqrt(variance / float64(len(pings)))

	return avgPing, jitter, nil
}

// getIPInfo retrieves IP and ISP information
func (c *Client) getIPInfo() (*IPInfo, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/getIP?isp=true", c.baseURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get IP info with status: %d", resp.StatusCode)
	}

	var ipInfo IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&ipInfo); err != nil {
		return nil, err
	}

	return &ipInfo, nil
}

// IPInfo represents the response from the getIP endpoint
type IPInfo struct {
	ProcessedString string `json:"processedString"`
	RawISPInfo      struct {
		Organization string `json:"organization"`
		Country      string `json:"country"`
		Location     string `json:"location"`
	} `json:"rawIspInfo"`
}

// Example usage:
func main() {
	client := NewClient("http://localhost:8989")
	result, err := client.RunTest()
	if err != nil {
		fmt.Printf("Speed test failed: %v\n", err)
		return
	}

	fmt.Printf("Speed Test Results:\n")
	fmt.Printf("IP: %s\n", result.IP)
	fmt.Printf("ISP: %s\n", result.ISP)
	fmt.Printf("Download: %.2f Mbps\n", result.DownloadSpeed)
	fmt.Printf("Upload: %.2f Mbps\n", result.UploadSpeed)
	fmt.Printf("Ping: %.2f ms\n", result.Ping)
	fmt.Printf("Jitter: %.2f ms\n", result.Jitter)
}
