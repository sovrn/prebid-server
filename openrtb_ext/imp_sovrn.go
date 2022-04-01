package openrtb_ext

type ExtImpSovrn struct {
	TagId          string  `json:"tagId,omitempty"`
	Tagid          string  `json:"tagid,omitempty"`
	BidFloor       float64 `json:"bidfloor"`
	Mimes          []int   `json:"mimes"`
	Minduration    int     `json:"minduration"`
	Maxduration    int     `json:"maxduration"`
	Protocols      []int   `json:"protocols"`
	W              int     `json:"w,omitempty"`
	H              int     `json:"h,omitempty"`
	Startdelay     int     `json:"startdelay,omitempty"`
	Placement      int     `json:"placement,omitempty"`
	Linearity      int     `json:"linearity,omitempty"`
	Skip           int     `json:"skip,omitempty"`
	Skipmin        int     `json:"skipmin,omitempty"`
	Skipafter      int     `json:"skipafter,omitempty"`
	Sequence       int     `json:"sequence,omitempty"`
	Battr          []int   `json:"battr,omitempty"`
	Maxextended    int     `json:"maxextended,omitempty"`
	Minbitrate     int     `json:"minbitrate,omitempty"`
	Maxbitrate     int     `json:"maxbitrate,omitempty"`
	Boxingallowed  int     `json:"boxingallowed,omitempty"`
	Playbackmethod []int   `json:"playbackmethod,omitempty"`
	Playbackend    int     `json:"playbackend,omitempty"`
	Delivery       []int   `json:"delivery,omitempty"`
	Pos            int     `json:"pos,omitempty"`
	Api            []int   `json:"api,omitempty"`
}
