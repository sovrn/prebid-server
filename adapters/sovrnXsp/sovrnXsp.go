package sovrnXsp

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/prebid/prebid-server/v2/adapters"
	"github.com/prebid/prebid-server/v2/config"
	"github.com/prebid/prebid-server/v2/errortypes"
	"github.com/prebid/prebid-server/v2/openrtb_ext"

	"github.com/prebid/openrtb/v19/openrtb2"
)

type adapter struct {
	Endpoint string
}

// bidExt.CreativeType values.
const (
	creativeTypeBanner int = 0
	creativeTypeVideo      = 1
	creativeTypeNative     = 2
	creativeTypeAudio      = 3
)

// Bid response extension from XSP.
type bidExt struct {
	CreativeType int `json:"creative_type"`
}

func (a *adapter) MakeRequests(request *openrtb2.BidRequest, requestInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	if request.App != nil {
		appCopy := *request.App
		if appCopy.Publisher == nil {
			appCopy.Publisher = &openrtb2.Publisher{}
		} else {
			publisherCopy := *appCopy.Publisher
			appCopy.Publisher = &publisherCopy
		}
		request.App = &appCopy
	} else {
		return nil, []error{&errortypes.BadInput{
			Message: "non-app request",
		}}
	}

	var errors []error
	var imps []openrtb2.Imp

	for idx, imp := range request.Imp {
		if imp.Banner == nil && imp.Video == nil && imp.Native == nil {
			continue
		}

		var bidderExt adapters.ExtImpBidder
		if err := json.Unmarshal(imp.Ext, &bidderExt); err != nil {
			err = &errortypes.BadInput{
				Message: fmt.Sprintf("imp #%d: ext.bidder not provided", idx),
			}
			errors = append(errors, err)
			continue
		}

		var xspExt openrtb_ext.ExtImpSovrnXsp
		if err := json.Unmarshal(bidderExt.Bidder, &xspExt); err != nil {
			err = &errortypes.BadInput{
				Message: fmt.Sprintf("imp #%d: %s", idx, err.Error()),
			}
			errors = append(errors, err)
			continue
		}

		if xspExt.MedID != "" {
			request.App.ID = xspExt.MedID
		}
		if xspExt.PubID != "" {
			request.App.Publisher.ID = xspExt.PubID
		}
		if xspExt.ZoneID != "" {
			imp.TagID = xspExt.ZoneID
		}
		imps = append(imps, imp)
	}

	if len(imps) == 0 {
		return nil, errors
	}

	request.Imp = imps
	requestJson, err := json.Marshal(request)
	if err != nil {
		return nil, append(errors, err)
	}

	headers := http.Header{}
	headers.Add("Content-Type", "application/json;charset=utf-8")
	headers.Add("Accept", "application/json")
	headers.Add("x-openrtb-version", "2.5")

	return []*adapters.RequestData{{
		Method:  "POST",
		Uri:     a.Endpoint,
		Body:    requestJson,
		Headers: headers,
	}}, errors
}

func (a *adapter) MakeBids(request *openrtb2.BidRequest, requestData *adapters.RequestData, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if responseData.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if responseData.StatusCode != http.StatusOK {
		return nil, []error{&errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d", responseData.StatusCode),
		}}
	}

	var response openrtb2.BidResponse
	if err := json.Unmarshal(responseData.Body, &response); err != nil {
		return nil, []error{err}
	}

	var errors []error
	result := adapters.NewBidderResponseWithBidsCapacity(len(request.Imp))

	for _, seatBid := range response.SeatBid {
		for _, bid := range seatBid.Bid {
			bid := bid
			var ext bidExt
			if err := json.Unmarshal(bid.Ext, &ext); err != nil {
				errors = append(errors, err)
				continue
			}

			var bidType openrtb_ext.BidType
			switch ext.CreativeType {
			case creativeTypeBanner:
				bidType = openrtb_ext.BidTypeBanner
			case creativeTypeVideo:
				bidType = openrtb_ext.BidTypeVideo
			case creativeTypeNative:
				bidType = openrtb_ext.BidTypeNative
			default:
				errors = append(errors, &errortypes.BadServerResponse{
					Message: fmt.Sprintf("Unsupported creative type: %d", ext.CreativeType),
				})
			}

			bid.Ext = nil
			result.Bids = append(result.Bids, &adapters.TypedBid{
				Bid:     &bid,
				BidType: bidType,
			})
		}
	}
	return result, errors
}

// Builder builds a new instance of the SovrnXSP adapter for the given bidder with the given config.
func Builder(bidderName openrtb_ext.BidderName, config config.Adapter, server config.Server) (adapters.Bidder, error) {
	bidder := &adapter{
		Endpoint: config.Endpoint,
	}
	return bidder, nil
}
