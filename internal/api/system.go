package api

import "net/url"

// RestartCore restarts the mihomo core (POST /restart). The socket connection
// briefly drops while the core comes back up.
func (c *Client) RestartCore() error {
	_, err := c.post("/restart", nil)
	return err
}

// UpgradeCore upgrades the mihomo core binary (POST /upgrade). Slow (~60s).
func (c *Client) UpgradeCore(channel string, force bool) error {
	q := url.Values{}
	if channel != "" {
		q.Set("channel", channel)
	}
	if force {
		q.Set("force", "true")
	}
	path := "/upgrade"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	_, err := c.post(path, nil)
	return err
}

// FlushFakeIP clears the fake-ip DNS cache (POST /cache/fakeip/flush).
func (c *Client) FlushFakeIP() error {
	_, err := c.post("/cache/fakeip/flush", nil)
	return err
}

// FlushDNS clears the DNS cache (POST /cache/dns/flush).
func (c *Client) FlushDNS() error {
	_, err := c.post("/cache/dns/flush", nil)
	return err
}

// UpdateGeo updates the GeoIP/GeoSite databases (POST /configs/geo). Slow.
func (c *Client) UpdateGeo() error {
	_, err := c.post("/configs/geo", nil)
	return err
}
