// Copyright © 2014-2016 Thomas Rabaix <thomas.rabaix@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package feed

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_FeedHandler(t *testing.T) {

	h := &FeedHandler{}

	data, _ := h.GetStruct()

	assert.IsType(t, &Feed{}, data)
}
