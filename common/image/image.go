package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/songquanpeng/one-api/common/client"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// Regex to match data URL pattern
var dataURLPattern = regexp.MustCompile(`data:image/([^;]+);base64,(.*)`)

// isPrivateIP reports whether ip falls into loopback, private, link-local or
// cloud-metadata ranges that must not be reachable from server-side fetches.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		privateRanges := []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"169.254.0.0/16", // includes 169.254.169.254 (cloud metadata)
			"100.64.0.0/10",
			"127.0.0.0/8",
		}
		for _, cidr := range privateRanges {
			_, netCIDR, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if netCIDR.Contains(v4) {
				return true
			}
		}
	}
	return false
}

// isBlockedURL returns true when the URL points to an internal/loopback address
// that must not be fetched server-side (SSRF protection). It fails closed.
// No domain name is hard-coded; only IP ranges are blocked.
func isBlockedURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return true
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return isPrivateIP(ip)
	}
	// DNS resolution: validate resolved IPs (defense against DNS rebinding).
	ips, err := net.LookupIP(host)
	if err != nil {
		return true
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

func IsImageUrl(url string) (bool, error) {
	if isBlockedURL(url) {
		return false, fmt.Errorf("blocked or invalid image URL")
	}
	resp, err := client.UserContentRequestHTTPClient.Head(url)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		return false, nil
	}
	return true, nil
}

func GetImageSizeFromUrl(url string) (width int, height int, err error) {
	isImage, err := IsImageUrl(url)
	if !isImage {
		return
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	img, _, err := image.DecodeConfig(resp.Body)
	if err != nil {
		return
	}
	return img.Width, img.Height, nil
}

func GetImageFromUrl(url string) (mimeType string, data string, err error) {
	// Check if the URL is a data URL
	matches := dataURLPattern.FindStringSubmatch(url)
	if len(matches) == 3 {
		// URL is a data URL
		mimeType = "image/" + matches[1]
		data = matches[2]
		return
	}

	isImage, err := IsImageUrl(url)
	if !isImage {
		return
	}
	resp, err := client.UserContentRequestHTTPClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	buffer := bytes.NewBuffer(nil)
	_, err = buffer.ReadFrom(resp.Body)
	if err != nil {
		return
	}
	mimeType = resp.Header.Get("Content-Type")
	data = base64.StdEncoding.EncodeToString(buffer.Bytes())
	return
}

var (
	reg = regexp.MustCompile(`data:image/([^;]+);base64,`)
)

var readerPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Reader{}
	},
}

func GetImageSizeFromBase64(encoded string) (width int, height int, err error) {
	decoded, err := base64.StdEncoding.DecodeString(reg.ReplaceAllString(encoded, ""))
	if err != nil {
		return 0, 0, err
	}

	reader := readerPool.Get().(*bytes.Reader)
	defer readerPool.Put(reader)
	reader.Reset(decoded)

	img, _, err := image.DecodeConfig(reader)
	if err != nil {
		return 0, 0, err
	}

	return img.Width, img.Height, nil
}

func GetImageSize(image string) (width int, height int, err error) {
	if strings.HasPrefix(image, "data:image/") {
		return GetImageSizeFromBase64(image)
	}
	return GetImageSizeFromUrl(image)
}
