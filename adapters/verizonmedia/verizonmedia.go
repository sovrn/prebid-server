package verizonmedia

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mxmCherry/openrtb"
	"github.com/prebid/prebid-server/adapters"
	"github.com/prebid/prebid-server/errortypes"
	"github.com/prebid/prebid-server/openrtb_ext"
)

type VerizonMediaAdapter struct {
	http *adapters.HTTPAdapter
	URI  string
}

func (a *VerizonMediaAdapter) Name() string {
	return "verizonmedia"
}

func (a *VerizonMediaAdapter) SkipNoCookies() bool {
	return false
}

func (a *VerizonMediaAdapter) MakeRequests(request *openrtb.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	errors := make([]error, 0, 1)

	if len(request.Imp) == 0 {
		err := &errortypes.BadInput{
			Message: "No impression in the bid request",
		}
		errors = append(errors, err)
		return nil, errors
	}

	reqs := make([]*adapters.RequestData, 0, len(request.Imp))
	headers := http.Header{}

	headers.Add("Content-Type", "application/json;charset=utf-8")
	headers.Add("Accept", "application/json")
	headers.Add("x-openrtb-version", "2.5")

	if request.Device != nil && request.Device.UA != "" {
		headers.Set("User-Agent", request.Device.UA)
	}

	for idx, imp := range request.Imp {
		var bidderExt adapters.ExtImpBidder
		err := json.Unmarshal(imp.Ext, &bidderExt)
		if err != nil {
			err = &errortypes.BadInput{
				Message: fmt.Sprintf("imp #%d: ext.bidder not provided", idx),
			}
			errors = append(errors, err)
			continue
		}

		var verizonMediaExt openrtb_ext.ExtImpVerizonMedia
		err = json.Unmarshal(bidderExt.Bidder, &verizonMediaExt)
		if err != nil {
			err = &errortypes.BadInput{
				Message: fmt.Sprintf("imp #%d: %s", idx, err.Error()),
			}
			errors = append(errors, err)
			continue
		}

		if verizonMediaExt.Dcn == "" {
			err = &errortypes.BadInput{
				Message: fmt.Sprintf("imp #%d: missing param dcn", idx),
			}
			errors = append(errors, err)
			continue
		}

		if verizonMediaExt.Pos == "" {
			err = &errortypes.BadInput{
				Message: fmt.Sprintf("imp #%d: missing param pos", idx),
			}
			errors = append(errors, err)
			continue
		}

		// Split up multi-impression requests into multiple requests so that
		// each split request is only associated to a single impression
		reqCopy := *request
		reqCopy.Imp = []openrtb.Imp{imp}

		siteCopy := *request.Site
		reqCopy.Site = &siteCopy

		if err := changeRequestForBidService(&reqCopy, &verizonMediaExt); err != nil {
			errors = append(errors, err)
			continue
		}

		reqJSON, err := json.Marshal(&reqCopy)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		reqs = append(reqs, &adapters.RequestData{
			Method:  "POST",
			Uri:     a.URI,
			Body:    reqJSON,
			Headers: headers,
		})
	}

	return reqs, errors
}

func (a *VerizonMediaAdapter) MakeBids(internalRequest *openrtb.BidRequest, externalRequest *adapters.RequestData, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {

	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if response.StatusCode != http.StatusOK {
		return nil, []error{&errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d.", response.StatusCode),
		}}
	}

	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(response.Body, &bidResp); err != nil {
		return nil, []error{&errortypes.BadServerResponse{
			Message: fmt.Sprintf("Bad server response: %d.", err),
		}}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(len(internalRequest.Imp))

	if len(bidResp.SeatBid) < 1 {
		return nil, []error{&errortypes.BadServerResponse{
			Message: fmt.Sprintf("Invalid SeatBids count: %d", len(bidResp.SeatBid)),
		}}
	}

	for _, sb := range bidResp.SeatBid {
		for _, bid := range sb.Bid {
			exists, mediaTypeId := getImpInfo(bid.ImpID, internalRequest.Imp)
			if !exists {
				return nil, []error{&errortypes.BadServerResponse{
					Message: fmt.Sprintf("Unknown ad unit code '%s'", bid.ImpID),
				}}
			}

			if openrtb_ext.BidTypeBanner != mediaTypeId {
				//only banner is supported, anything else is ignored
				continue
			}

			bidResponse.Bids = append(bidResponse.Bids, &adapters.TypedBid{
				Bid:     &bid,
				BidType: openrtb_ext.BidTypeBanner,
			})
		}
	}

	return bidResponse, nil
}

func getImpInfo(impId string, imps []openrtb.Imp) (bool, openrtb_ext.BidType) {
	var mediaType openrtb_ext.BidType
	var exists bool
	for _, imp := range imps {
		if imp.ID == impId {
			exists = true
			if imp.Banner != nil {
				mediaType = openrtb_ext.BidTypeBanner
			}
			break
		}
	}
	return exists, mediaType
}

func changeRequestForBidService(request *openrtb.BidRequest, extension *openrtb_ext.ExtImpVerizonMedia) error {
	/* Always override the tag ID and site ID of the request */
	request.Imp[0].TagID = extension.Pos
	request.Site.ID = extension.Dcn

	if request.Imp[0].Banner == nil {
		return nil
	}

	banner := *request.Imp[0].Banner
	request.Imp[0].Banner = &banner

	if banner.W != nil && banner.H != nil {
		if *banner.W == 0 || *banner.H == 0 {
			return errors.New(fmt.Sprintf("Invalid sizes provided for Banner %dx%d", *banner.W, *banner.H))
		}
		return nil
	}

	if len(banner.Format) == 0 {
		return errors.New(fmt.Sprintf("No sizes provided for Banner %v", banner.Format))
	}

	banner.W = new(uint64)
	*banner.W = banner.Format[0].W
	banner.H = new(uint64)
	*banner.H = banner.Format[0].H

	return nil
}

func NewVerizonMediaAdapter(config *adapters.HTTPAdapterConfig, uri string) *VerizonMediaAdapter {
	a := adapters.NewHTTPAdapter(config)

	return &VerizonMediaAdapter{
		http: a,
		URI:  uri,
	}
}

func NewVerizonMediaBidder(client *http.Client, endpoint string) *VerizonMediaAdapter {
	a := &adapters.HTTPAdapter{Client: client}
	return &VerizonMediaAdapter{
		http: a,
		URI:  endpoint,
	}
}