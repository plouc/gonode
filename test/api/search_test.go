// Copyright © 2014-2015 Thomas Rabaix <thomas.rabaix@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/rande/goapp"
	"github.com/rande/gonode/core"
	"github.com/rande/gonode/plugins/api"
	"github.com/rande/gonode/test"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"os"
	"testing"
)

func CheckNoResults(t *testing.T, p *api.ApiPager) {
	assert.Equal(t, uint64(32), p.PerPage)
	assert.Equal(t, uint64(1), p.Page)
	assert.Equal(t, 0, len(p.Elements))
	assert.Equal(t, uint64(0), p.Next)
	assert.Equal(t, uint64(0), p.Previous)
}

func Test_Search_Basic(t *testing.T) {
	values := []struct {
		Url string
		Len int
	}{
		{"/nodes", 2},
		{"/nodes?type=core.user", 2},
		{"/nodes?type=core.user&data.username=user12", 1},
		{"/nodes?type=core.user&data.username=user12&data.username=user13", 1},
		{"/nodes?&page=-1&page=1", 2}, // the last occurrence erase first values
	}

	for _, v := range values {
		test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
			// WITH
			auth := test.GetAuthHeader(t, ts)
			file, _ := os.Open("../fixtures/new_user.json")
			test.RunRequest("POST", ts.URL+"/nodes", file, auth)

			// WHEN
			res, _ := test.RunRequest("GET", ts.URL+v.Url, nil, auth)

			p := GetPager(app, res)

			// THEN
			assert.Equal(t, uint64(32), p.PerPage)
			assert.Equal(t, uint64(1), p.Page)
			assert.Equal(t, v.Len, len(p.Elements))
			assert.Equal(t, uint64(0), p.Next)
			assert.Equal(t, uint64(0), p.Previous)

			n := p.Elements[0].(*core.Node)

			assert.Equal(t, "core.user", n.Type)
			assert.False(t, n.Deleted)
		})
	}
}

func Test_Search_NoResult(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		// WITH
		auth := test.GetAuthHeader(t, ts)
		file, _ := os.Open("../fixtures/new_user.json")
		test.RunRequest("POST", ts.URL+"/nodes", file)

		// WHEN
		res, _ := test.RunRequest("GET", ts.URL+"/nodes?type=other", nil, auth)

		p := GetPager(app, res)

		// THEN
		CheckNoResults(t, p)
	})
}

func Test_Search_Invalid_Pagination(t *testing.T) {
	urls := []string{
		"/nodes?per_page=-1",
		"/nodes?per_page=-1&page=-1",
		"/nodes?per_page=256",
		"/nodes?per_page=256&page=1",
		"/nodes?per_page=127&page=1&page=-1",
		// "/nodes?per_page=127&page=-1&page=1", // the last occurrence erase first values
	}

	for _, url := range urls {
		test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
			// WITH
			auth := test.GetAuthHeader(t, ts)
			file, _ := os.Open("../fixtures/new_user.json")
			test.RunRequest("POST", ts.URL+"/nodes", file, auth)

			// WHEN
			res, _ := test.RunRequest("GET", ts.URL+url, nil, auth)

			assert.Equal(t, 412, res.StatusCode, "url: "+url)
		})
	}
}

func Test_Search_Invalid_OrderBy(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)

		// seems goji or golang block this request
		res, _ := test.RunRequest("GET", ts.URL+"/nodes?order_by=\"1 = 1\"; DELETE * FROM node,ASC", nil, auth)
		assert.Equal(t, 400, res.StatusCode, "url: /nodes?order_by=\"1 = 1\"; DELETE * FROM node,ASC")

		// seems goji or golang block this request
		res, _ = test.RunRequest("GET", ts.URL+"/nodes?order_by=DELETE%20*%20FROM%20node,ASC", nil, auth)
		assert.Equal(t, 412, res.StatusCode, "url: /nodes?order_by=DELETE%20*%20FROM%20node,ASC")
	})
}

func Test_Search_OrderBy_Name_ASC(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)
		InitSearchFixture(app)

		res, _ := test.RunRequest("GET", ts.URL+"/nodes?order_by=name,ASC", nil, auth)

		assert.Equal(t, 200, res.StatusCode, "url: /nodes?order_by=name,ASC")

		p := GetPager(app, res)

		assert.Equal(t, 4, len(p.Elements))
		assert.Equal(t, "User A", p.Elements[0].(*core.Node).Name)
		assert.Equal(t, "User AA", p.Elements[1].(*core.Node).Name)
		assert.Equal(t, "User B", p.Elements[2].(*core.Node).Name)
	})
}

func Test_Search_OrderBy_Name_DESC(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)
		InitSearchFixture(app)

		res, _ := test.RunRequest("GET", ts.URL+"/nodes?order_by=name,DESC", nil, auth)

		assert.Equal(t, 200, res.StatusCode, "url: /nodes?order_by=name,DESC")

		p := GetPager(app, res)

		assert.Equal(t, 4, len(p.Elements))
		assert.Equal(t, "User ZZ", p.Elements[0].(*core.Node).Name)
		assert.Equal(t, "User B", p.Elements[1].(*core.Node).Name)
		assert.Equal(t, "User AA", p.Elements[2].(*core.Node).Name)
	})
}

func Test_Search_OrderBy_Weight_DESC_Name_ASC(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)
		InitSearchFixture(app)

		// TESTING WITH 2 ORDERING OPTION
		res, _ := test.RunRequest("GET", ts.URL+"/nodes?order_by=weight,DESC&order_by=name,ASC", nil, auth)

		assert.Equal(t, 200, res.StatusCode, "url: /nodes?order_by=weight,DESC&order_by=name,ASC")

		p := GetPager(app, res)

		assert.Equal(t, 4, len(p.Elements))
		assert.Equal(t, "User AA", p.Elements[0].(*core.Node).Name)
		assert.Equal(t, "User A", p.Elements[1].(*core.Node).Name)
		assert.Equal(t, "User B", p.Elements[2].(*core.Node).Name)
	})
}

func Test_Search_OrderBy_Meta_Username(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)
		InitSearchFixture(app)

		// TESTING WITH 2 ORDERING OPTION
		res, _ := test.RunRequest("GET", ts.URL+"/nodes?order_by=meta.username,DESC", nil, auth)

		assert.Equal(t, 200, res.StatusCode, "url: /nodes?order_by=meta.username")

		p := GetPager(app, res)

		assert.Equal(t, 4, len(p.Elements))
		assert.Equal(t, "User ZZ", p.Elements[0].(*core.Node).Name)
		assert.Equal(t, "User A", p.Elements[1].(*core.Node).Name)
		assert.Equal(t, "User AA", p.Elements[2].(*core.Node).Name)
	})
}

func Test_Search_OrderBy_Meta_Non_Existant_Meta(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)
		InitSearchFixture(app)

		res, _ := test.RunRequest("GET", ts.URL+"/nodes?meta.username.fake=foo&order_by=meta.username.fake,DESC", nil, auth)

		assert.Equal(t, 200, res.StatusCode, "url: /nodes?order_by=meta.username.fake")

		p := GetPager(app, res)

		assert.Equal(t, 0, len(p.Elements))
	})
}

func Test_Search_Meta(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)
		InitSearchFixture(app)

		res, _ := test.RunRequest("GET", ts.URL+"/nodes?data.username=user-a", nil, auth)

		assert.Equal(t, 200, res.StatusCode, "url: /nodes?data.username=user-a")

		p := GetPager(app, res)

		assert.Equal(t, 1, len(p.Elements))
	})
}

func Test_Search_Slug(t *testing.T) {
	test.RunHttpTest(t, func(t *testing.T, ts *httptest.Server, app *goapp.App) {
		auth := test.GetAuthHeader(t, ts)
		InitSearchFixture(app)

		res, _ := test.RunRequest("GET", ts.URL+"/nodes?slug=user-a", nil, auth)

		assert.Equal(t, 200, res.StatusCode, "url: /nodes?slug=user-a")

		p := GetPager(app, res)

		assert.Equal(t, 1, len(p.Elements))
	})
}
