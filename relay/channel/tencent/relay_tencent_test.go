package tencent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTencentHandlerPreservesStringErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"Response":{"Error":{"Code":"InvalidParameter","Message":"Temperature must be 2 or less"}}}`)),
	}

	usage, apiError := tencentHandler(c, &relaycommon.RelayInfo{}, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiError)
	require.Equal(t, types.ErrorCode("InvalidParameter"), apiError.GetErrorCode())
	require.Equal(t, "Temperature must be 2 or less", apiError.Error())
}
