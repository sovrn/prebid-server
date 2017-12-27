package sovrn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/mxmCherry/openrtb"
	"github.com/prebid/prebid-server/adapters"
	"github.com/prebid/prebid-server/openrtb_ext"
	"github.com/prebid/prebid-server/pbs"
	"golang.org/x/net/context/ctxhttp"
)

type SovrnAdapter struct {
	http         *adapters.HTTPAdapter
	URI          string
	usersyncInfo *pbs.UsersyncInfo
}

// Name - export adapter name */
func (s *SovrnAdapter) Name() string {
	return "sovrn"
}

// FamilyName used for cookies and such
func (s *SovrnAdapter) FamilyName() string {
	return "sovrn"
}

// GetUsersyncInfo get the UsersyncInfo object defining sovrn user sync parameters
func (s *SovrnAdapter) GetUsersyncInfo() *pbs.UsersyncInfo {
	return s.usersyncInfo
}

type sovrnParams struct {
	TagID int `json:"tagid"`
}

func (s *SovrnAdapter) SkipNoCookies() bool {
	return false
}

// Call send bid requests to sovrn and receive responses
func (s *SovrnAdapter) Call(ctx context.Context, req *pbs.PBSRequest, bidder *pbs.PBSBidder) (pbs.PBSBidSlice, error) {
	supportedMediaTypes := []pbs.MediaType{pbs.MEDIA_TYPE_BANNER}
	sReq, err := adapters.MakeOpenRTBGeneric(req, bidder, s.FamilyName(), supportedMediaTypes, true)

	if err != nil {
		return nil, err
	}

	sovrnReq := openrtb.BidRequest{
		ID:   sReq.ID,
		Imp:  sReq.Imp,
		Site: sReq.Site,
	}

	// add tag ids to impressions
	for i, unit := range bidder.AdUnits {
		var params sovrnParams
		err = json.Unmarshal(unit.Params, &params)
		if err != nil {
			return nil, err
		}
		sovrnReq.Imp[i].Secure = sReq.Imp[i].Secure
		sovrnReq.Imp[i].TagID = strconv.Itoa(params.TagID)
		sovrnReq.Imp[i].Banner.Format = nil
	}

	reqJSON, err := json.Marshal(sovrnReq)
	if err != nil {
		return nil, err
	}

	debug := &pbs.BidderDebug{
		RequestURI: s.URI,
	}

	if req.IsDebug {
		debug.RequestBody = string(reqJSON)
		bidder.Debug = append(bidder.Debug, debug)
	}

	httpReq, _ := http.NewRequest("POST", s.URI, bytes.NewReader(reqJSON))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", sReq.Device.UA)
	httpReq.Header.Set("X-Forwarded-For", sReq.Device.IP)
	httpReq.Header.Set("Accept-Language", sReq.Device.Language)
	httpReq.Header.Set("DNT", strconv.Itoa(int(sReq.Device.DNT)))

	userID := strings.TrimSpace(sReq.User.BuyerUID)
	if len(userID) > 0 {
		httpReq.AddCookie(&http.Cookie{Name: "ljt_reader", Value: userID})
	}
	sResp, err := ctxhttp.Do(ctx, s.http.Client, httpReq)
	if err != nil {
		return nil, err
	}

	debug.StatusCode = sResp.StatusCode

	if sResp.StatusCode == 204 {
		return nil, nil
	}

	defer sResp.Body.Close()
	body, err := ioutil.ReadAll(sResp.Body)
	if err != nil {
		return nil, err
	}
	responseBody := string(body)

	if sResp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP status %d; body: %s", sResp.StatusCode, responseBody)
	}

	if req.IsDebug {
		debug.ResponseBody = responseBody
	}

	var bidResp openrtb.BidResponse
	err = json.Unmarshal(body, &bidResp)
	if err != nil {
		return nil, err
	}

	bids := make(pbs.PBSBidSlice, 0)

	for _, sb := range bidResp.SeatBid {
		for _, bid := range sb.Bid {
			bidID := bidder.LookupBidID(bid.ImpID)
			if bidID == "" {
				return nil, fmt.Errorf("Unknown ad unit code '%s'", bid.ImpID)
			}

			adm, _ := url.QueryUnescape(bid.AdM)
			pbid := pbs.PBSBid{
				BidID:       bidID,
				AdUnitCode:  bid.ImpID,
				BidderCode:  bidder.BidderCode,
				Price:       bid.Price,
				Adm:         adm,
				Creative_id: bid.CrID,
				Width:       bid.W,
				Height:      bid.H,
				DealId:      bid.DealID,
				NURL:        bid.NURL,
			}
			bids = append(bids, &pbid)
		}
	}

	sort.Sort(bids)
	return bids, nil
}

func (s *SovrnAdapter) MakeRequests(request *openrtb.BidRequest) ([]*adapters.RequestData, []error) {
	errs := make([]error, 0, len(request.Imp))

	for i := 0; i < len(request.Imp); i++ {
		_, err := preprocess(&request.Imp[i])
		if err != nil {
			errs = append(errs, err)
			request.Imp = append(request.Imp[:i], request.Imp[i+1:]...)
			i--
		}
	}

	// If all the requests were malformed, don't bother making a server call with no impressions.
	if len(request.Imp) == 0 {
		return nil, errs
	}

	reqJSON, err := json.Marshal(request)
	if err != nil {
		errs = append(errs, err)
		return nil, errs
	}

	headers := http.Header{}
	headers.Add("Content-Type", "application/json")
	headers.Add("User-Agent", request.Device.UA)
	headers.Add("X-Forwarded-For", request.Device.IP)
	headers.Add("Accept-Language", request.Device.Language)
	headers.Add("DNT", strconv.Itoa(int(request.Device.DNT)))

	userID := strings.TrimSpace(request.User.BuyerUID)
	if len(userID) > 0 {
		headers.Add("Cookie", fmt.Sprintf("%s=%s", "ljt_reader", userID))
	}

	return []*adapters.RequestData{{
		Method:  "POST",
		Uri:     s.URI,
		Body:    reqJSON,
		Headers: headers,
	}}, errs
}

func (s *SovrnAdapter) MakeBids(internalRequest *openrtb.BidRequest, externalRequest *adapters.RequestData, response *adapters.ResponseData) ([]*adapters.TypedBid, []error) {
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if response.StatusCode != http.StatusOK {
		return nil, []error{fmt.Errorf("Unexpected status code: %d. Run with request.debug = 1 for more info", response.StatusCode)}
	}

	var bidResp openrtb.BidResponse
	if err := json.Unmarshal(response.Body, &bidResp); err != nil {
		return nil, []error{err}
	}

	bids := make([]*adapters.TypedBid, 0, 5)

	for _, sb := range bidResp.SeatBid {
		for _, bid := range sb.Bid {
			bids = append(bids, &adapters.TypedBid{
				Bid:     &bid,
				BidType: getMediaTypeForImp(bid.ImpID, internalRequest.Imp),
			})
		}
	}

	return bids, nil
}

func preprocess(imp *openrtb.Imp) (string, error) {
	// We only support banner and video impressions for now.
	if imp.Native != nil || imp.Audio != nil || imp.Video != nil {
		return "", fmt.Errorf("Sovrn doesn't support audio, video, or native Imps. Ignoring Imp ID=%s", imp.ID)
	}

	var bidderExt adapters.ExtImpBidder
	if err := json.Unmarshal(imp.Ext, &bidderExt); err != nil {
		return "", err
	}

	var sovrnExt openrtb_ext.ExtImpSovrn
	if err := json.Unmarshal(bidderExt.Bidder, &sovrnExt); err != nil {
		return "", err
	}

	imp.TagID = strconv.Itoa(sovrnExt.TagId)
	imp.BidFloor = sovrnExt.BidFloor
	imp.Banner.Format = nil

	return imp.TagID, nil
}

func getMediaTypeForImp(impId string, imps []openrtb.Imp) openrtb_ext.BidType {
	for _, imp := range imps {
		if imp.ID == impId && imp.Video != nil {
			return openrtb_ext.BidTypeVideo
		}
	}
	return openrtb_ext.BidTypeBanner
}

// NewSovrnAdapter create a new SovrnAdapter instance
func NewSovrnAdapter(config *adapters.HTTPAdapterConfig, endpoint string, usersyncURL string, externalURL string) *SovrnAdapter {
	return NewSovrnBidder(adapters.NewHTTPAdapter(config).Client, endpoint, usersyncURL, externalURL)
}

func NewSovrnBidder(client *http.Client, endpoint string, usersyncURL string, externalURL string) *SovrnAdapter {
	a := &adapters.HTTPAdapter{Client: client}

	redirectURI := fmt.Sprintf("%s/setuid?bidder=sovrn&uid=$UID", externalURL)

	info := &pbs.UsersyncInfo{
		URL:         fmt.Sprintf("%sredir=%s", usersyncURL, url.QueryEscape(redirectURI)),
		Type:        "redirect",
		SupportCORS: false,
	}

	return &SovrnAdapter{
		http:         a,
		URI:          endpoint,
		usersyncInfo: info,
	}
}
