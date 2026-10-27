package api

// GetConfigRaw returns the body of GET /configs unparsed, so callers can see
// fields the Config struct does not model. The body carries inbound
// credentials (authentication, ss-config, vmess-config, tuic-server users);
// callers must filter it before printing or saving any of it.
func (c *Client) GetConfigRaw() ([]byte, error) {
	return c.get("/configs")
}
