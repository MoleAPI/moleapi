package openai

import (
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	sendChan := make(chan []byte, 100)
	receiveChan := make(chan []byte, 100)
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}
	var stateMu sync.Mutex
	var readers sync.WaitGroup
	readers.Add(2)

	gopool.Go(func() {
		defer readers.Done()
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}
				err = func() error {
					stateMu.Lock()
					defer stateMu.Unlock()
					if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate {
						if realtimeEvent.Session != nil {
							if realtimeEvent.Session.Tools != nil {
								info.RealtimeTools = realtimeEvent.Session.Tools
							}
						}
					}

					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						return fmt.Errorf("error counting text token: %v", err)
					}
					logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					localUsage.TotalTokens += textToken + audioToken
					localUsage.InputTokens += textToken + audioToken
					localUsage.InputTokenDetails.TextTokens += textToken
					localUsage.InputTokenDetails.AudioTokens += audioToken

					return nil
				}()
				if err != nil {
					errChan <- err
					return
				}

				err = helper.WssString(c, targetConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}

				select {
				case sendChan <- message:
				default:
				}
			}
		}
	})

	gopool.Go(func() {
		defer readers.Done()
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				err = func() error {
					stateMu.Lock()
					defer stateMu.Unlock()
					info.SetFirstResponseTime()
					realtimeEvent := &dto.RealtimeEvent{}
					err = common.Unmarshal(message, realtimeEvent)
					if err != nil {
						return fmt.Errorf("error unmarshalling message: %v", err)
					}
					if realtimeEvent.Type == dto.RealtimeEventTypeError {
						rawError := string(message)
						statusCode := 400
						if realtimeEvent.Error != nil {
							rawError = realtimeEvent.Error.Message + " " + fmt.Sprint(realtimeEvent.Error.Code)
							errorType := realtimeEvent.Error.Type
							if errorType == "server_error" || errorType == "api_error" || errorType == "overloaded_error" {
								statusCode = 500
							} else if errorType == "rate_limit_error" {
								statusCode = 429
							}
						}
						logger.LogWarn(c, "masked upstream realtime error from user response: "+common.LocalLogPreview(rawError))
						if info.StreamStatus == nil {
							info.StreamStatus = relaycommon.NewStreamStatus()
						}
						info.StreamStatus.RecordError(rawError)
						_, publicError := service.PublicUpstreamError(statusCode, rawError)
						message, err = common.Marshal(dto.RealtimeEvent{EventId: realtimeEvent.EventId, Type: dto.RealtimeEventTypeError, Error: &publicError})
						if err != nil {
							return fmt.Errorf("error marshalling public realtime error: %v", err)
						}
					}

					if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
						var realtimeUsage *dto.RealtimeUsage
						if realtimeEvent.Response != nil {
							realtimeUsage = realtimeEvent.Response.Usage
						}
						if realtimeUsage != nil {
							usage = realtimeUsage
							localUsage = &dto.RealtimeUsage{}
							err := preConsumeUsage(c, info, usage, sumUsage)
							if err != nil {
								return fmt.Errorf("error consume usage: %v", err)
							}
							// 本次计费完成，清除
							usage = &dto.RealtimeUsage{}

						} else {
							textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
							if err != nil {
								return fmt.Errorf("error counting text token: %v", err)
							}
							logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
							localUsage.TotalTokens += textToken + audioToken
							info.IsFirstRequest = false
							localUsage.InputTokens += textToken + audioToken
							localUsage.InputTokenDetails.TextTokens += textToken
							localUsage.InputTokenDetails.AudioTokens += audioToken
							err = preConsumeUsage(c, info, localUsage, sumUsage)
							if err != nil {
								return fmt.Errorf("error consume usage: %v", err)
							}
							// 本次计费完成，清除
							localUsage = &dto.RealtimeUsage{}
							// print now usage
						}
						logger.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", sumUsage))
						logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))
						logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))

					} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
						realtimeSession := realtimeEvent.Session
						if realtimeSession != nil {
							// update audio format
							info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
							info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
						}
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							return fmt.Errorf("error counting text token: %v", err)
						}
						logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						localUsage.OutputTokens += textToken + audioToken
						localUsage.OutputTokenDetails.TextTokens += textToken
						localUsage.OutputTokenDetails.AudioTokens += audioToken
					}

					return nil
				}()
				if err != nil {
					errChan <- err
					return
				}

				err = helper.WssString(c, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				select {
				case receiveChan <- message:
				default:
				}
			}
		}
	})

	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		//return service.OpenAIErrorWrapper(err, "realtime_error", http.StatusInternalServerError), nil
		logger.LogError(c, "realtime error: "+err.Error())
	case <-c.Done():
	}

	_ = clientConn.Close()
	_ = targetConn.Close()
	readers.Wait()

	if usage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, usage, sumUsage)
	}

	if localUsage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	// check usage total tokens, if 0, use local usage

	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	for _, counts := range [][2]*int{
		{&totalUsage.TotalTokens, &usage.TotalTokens}, {&totalUsage.InputTokens, &usage.InputTokens}, {&totalUsage.OutputTokens, &usage.OutputTokens},
		{&totalUsage.InputTokenDetails.CachedTokens, &usage.InputTokenDetails.CachedTokens},
		{&totalUsage.InputTokenDetails.TextTokens, &usage.InputTokenDetails.TextTokens},
		{&totalUsage.InputTokenDetails.AudioTokens, &usage.InputTokenDetails.AudioTokens},
		{&totalUsage.InputTokenDetails.ImageTokens, &usage.InputTokenDetails.ImageTokens},
		{&totalUsage.OutputTokenDetails.TextTokens, &usage.OutputTokenDetails.TextTokens},
		{&totalUsage.OutputTokenDetails.AudioTokens, &usage.OutputTokenDetails.AudioTokens},
	} {
		value, clamp := common.QuotaRoundChecked(float64(max(*counts[0], 0)) + float64(max(*counts[1], 0)))
		*counts[0] = value
		if clamp != nil {
			info.QuotaClamp = clamp
		}
	}
	if incoming := usage.InputTokenDetails.CachedTokensDetails; incoming != nil {
		if totalUsage.InputTokenDetails.CachedTokensDetails == nil {
			totalUsage.InputTokenDetails.CachedTokensDetails = &dto.CachedTokenDetails{}
		}
		total := totalUsage.InputTokenDetails.CachedTokensDetails
		for _, field := range []struct {
			target **int
			value  *int
		}{
			{&total.TextTokens, incoming.TextTokens}, {&total.AudioTokens, incoming.AudioTokens}, {&total.ImageTokens, incoming.ImageTokens},
		} {
			if field.value == nil {
				continue
			}
			if *field.target == nil {
				*field.target = new(int)
			}
			value, clamp := common.QuotaRoundChecked(float64(max(**field.target, 0)) + float64(max(*field.value, 0)))
			**field.target = value
			if clamp != nil {
				info.QuotaClamp = clamp
			}
		}
	}
	// Consume each response once even if reserving its cost fails.
	*usage = dto.RealtimeUsage{}
	return service.PreWssConsumeQuota(ctx, info, totalUsage)
}
