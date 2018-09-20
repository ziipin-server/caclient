package caclient

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"github.com/robertkrimen/otto"
	"github.com/ziipin-server/niuhe"
)

type CheckMockFunc func(url string) (needMock bool, jsPath string)

type ApiCall struct {
	url       string
	form      url.Values
	checkMock CheckMockFunc
}

func NewCallWithCheckMock(apiURL string, checkMock CheckMockFunc) *ApiCall {
	return &ApiCall{
		url:       apiURL,
		form:      url.Values{},
		checkMock: checkMock,
	}
}

func NewCall(apiURL string) *ApiCall {
	return NewCallWithCheckMock(apiURL, nil)
}

func (c *ApiCall) StrArg(key string, values ...string) *ApiCall {
	for _, val := range values {
		c.form.Add(key, val)
	}
	return c
}

func (c *ApiCall) Arg(key string, values ...interface{}) *ApiCall {
	for _, val := range values {
		switch v := val.(type) {
		case string:
			c.form.Add(key, v)
		case int:
			c.form.Add(key, strconv.Itoa(v))
		default:
			c.form.Add(key, fmt.Sprint(v))
		}
	}
	return c
}

func (c *ApiCall) execMock(jspath string, dataPtr interface{}) (bool, error) {
	codeBytes, err := ioutil.ReadFile(jspath)
	if err != nil {
		return true, err
	}
	vm := otto.New()
	vm.Set("url", c.url)
	vm.Set("form", c.form)
	rv, err := vm.Run("JSON.stringify((" + string(codeBytes) + ")(url, form))")
	if err != nil {
		return true, err
	}
	if rv.IsUndefined() {
		return false, nil
	}
	rs, err := rv.ToString()
	if err != nil {
		return true, err
	}
	var rsp struct {
		Result  int         `json:"result"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}
	rsp.Data = dataPtr
	if err := json.Unmarshal([]byte(rs), &rsp); err != nil {
		return true, err
	}
	if rsp.Result != 0 {
		return true, niuhe.NewCommError(rsp.Result, rsp.Message)
	}
	return true, nil
}

func (c *ApiCall) Exec(dataPtr interface{}) error {
	if c.checkMock != nil {
		needMock, jsPath := c.checkMock(c.url)
		if needMock {
			mocked, err := c.execMock(jsPath, dataPtr)
			if mocked {
				return err
			}
		}
	}
	resp, err := http.PostForm(c.url, c.form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	var wrapper struct {
		Result  int         `json:"result"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}
	wrapper.Data = dataPtr
	if err := dec.Decode(&wrapper); err != nil {
		return err
	}
	if wrapper.Result != 0 {
		return niuhe.NewCommError(wrapper.Result, wrapper.Message)
	}
	return nil
}
