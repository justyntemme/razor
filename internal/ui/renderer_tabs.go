package ui

// Browser tab management

// EnableTabs enables the browser tab bar
func (r *Renderer) EnableTabs(enabled bool) {
	r.tabsEnabled = enabled
	if enabled && len(r.browserTabs) == 0 {
		// Initialize with one tab
		r.browserTabs = []BrowserTab{{
			ID:    "tab-0",
			Title: "Home",
			Path:  "",
		}}
		r.activeTabIndex = 0
	}
}

// AddTab adds a new browser tab and returns its index
func (r *Renderer) AddTab(id, title, path string) int {
	r.browserTabs = append(r.browserTabs, BrowserTab{
		ID:    id,
		Title: title,
		Path:  path,
	})
	return len(r.browserTabs) - 1
}

// CloseTab removes a tab by index, returns the new active index
func (r *Renderer) CloseTab(index int) int {
	if index < 0 || index >= len(r.browserTabs) || len(r.browserTabs) <= 1 {
		return r.activeTabIndex
	}
	r.browserTabs = append(r.browserTabs[:index], r.browserTabs[index+1:]...)
	if r.activeTabIndex >= len(r.browserTabs) {
		r.activeTabIndex = len(r.browserTabs) - 1
	} else if r.activeTabIndex > index {
		r.activeTabIndex--
	}
	return r.activeTabIndex
}

// SetActiveTab sets the active tab index
func (r *Renderer) SetActiveTab(index int) {
	if index >= 0 && index < len(r.browserTabs) {
		r.activeTabIndex = index
	}
}

// UpdateTabTitle updates the title of a tab
func (r *Renderer) UpdateTabTitle(index int, title string) {
	if index >= 0 && index < len(r.browserTabs) {
		r.browserTabs[index].Title = title
	}
}

// UpdateTabPath updates the path of a tab
func (r *Renderer) UpdateTabPath(index int, path string) {
	if index >= 0 && index < len(r.browserTabs) {
		r.browserTabs[index].Path = path
	}
}

// GetActiveTabIndex returns the current active tab index
func (r *Renderer) GetActiveTabIndex() int {
	return r.activeTabIndex
}

// GetTabCount returns the number of tabs
func (r *Renderer) GetTabCount() int {
	return len(r.browserTabs)
}

// TabsEnabled returns whether tabs are enabled
func (r *Renderer) TabsEnabled() bool {
	return r.tabsEnabled
}
