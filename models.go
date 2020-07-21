package main

type YTSearch struct {
	Kind          string `json:"kind"`
	NextPageToken string `json:"nextPageToken"`
	Items         []struct {
		Kind string `json:"kind"`
		ID   struct {
			Kind    string `json:"kind"`
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Thumbnails  struct {
				Default struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"default"`
				Medium struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"medium"`
				High struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"high"`
			} `json:"thumbnails"`
		} `json:"snippet"`
	} `json:"items"`
}


type Temp struct {
	Title        string   `json:"title"`
	Poster       []string `json:"poster"`
	Portriat     []string `json:"portriat"`
	IsDetailPage bool     `json:"isDetailPage"`
	ContentID    string   `json:"contentId"`
	Package      string   `json:"package"`
	Source       string   `json:"source"`
	Contenttype  string   `json:"contenttype"`
	Target       []string `json:"target"`
}


