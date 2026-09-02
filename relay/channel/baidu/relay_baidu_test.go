package baidu

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

func TestBaiduHandlerPreservesProviderError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"error_code":336103,"error_msg":"Prompt tokens too long"}`)),
	}

	apiError, usage := baiduHandler(c, &relaycommon.RelayInfo{}, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiError)
	require.Equal(t, http.StatusBadRequest, apiError.StatusCode)
	require.Equal(t, types.ErrorCode("336103"), apiError.GetErrorCode())
	require.Equal(t, "Prompt tokens too long", apiError.Error())
}
