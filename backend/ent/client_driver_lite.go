package ent

import "entgo.io/ent/dialect"

// Driver exposes the generated client's dialect driver to retained raw-SQL helpers.
func (c *Client) Driver() dialect.Driver {
	if c == nil {
		return nil
	}
	return c.driver
}
