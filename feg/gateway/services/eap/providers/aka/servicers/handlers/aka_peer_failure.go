/*
Copyright (c) Facebook, Inc. and its affiliates.
All rights reserved.

This source code is licensed under the BSD-style license found in the
LICENSE file in the root directory of this source tree.
*/

// Package handlers provided AKA Response handlers for supported AKA subtypes
package handlers

import (
	"fmt"
	"log"

	"magma/feg/gateway/services/eap"
	"magma/feg/gateway/services/eap/protos"
	"magma/feg/gateway/services/eap/providers/aka"
	"magma/feg/gateway/services/eap/providers/aka/servicers"
)

func init() {
	servicers.AddHandler(aka.SubtypeAuthenticationReject, authRejectResponse)
	servicers.AddHandler(aka.SubtypeClientError, clientErrorResponse)
	servicers.AddHandler(aka.SubtypeNotification, notificationResponse)
}

// authRejectResponse implements handler for EAP-Response/AKA-Authentication-Reject,
// see https://tools.ietf.org/html/rfc4187#section-9.5 for details
func authRejectResponse(s *servicers.EapAkaSrv, ctx *protos.EapContext, req eap.Packet) (eap.Packet, error) {
	var sid string

	if ctx == nil || len(ctx.SessionId) == 0 {
		log.Printf("WARNING: Missing CTX/Empty Session ID in AKA-Authentication-Reject")
	} else {
		sid = ctx.SessionId
	}
	return peerFailure(s, sid, req.Identifier(), 0), nil
}

// string implements handler for EAP-Response/AKA-Client-Error,
// see https://tools.ietf.org/html/rfc4187#section-9.9 for details
func clientErrorResponse(s *servicers.EapAkaSrv, ctx *protos.EapContext, req eap.Packet) (eap.Packet, error) {
	var (
		sid       string
		resultErr error
		errorCode int
	)
	if ctx != nil && len(ctx.SessionId) > 0 {
		sid = ctx.SessionId
		scanner, err := eap.NewAttributeScanner(req)
		if err != nil {
			resultErr = fmt.Errorf("Malformed AKA-Client-Error Packet %v", err)
			log.Printf("WARNING: %v", resultErr)
		} else {
			var a eap.Attribute
			for a, err = scanner.Next(); err == nil; a, err = scanner.Next() {
				if a.Type() == aka.AT_CLIENT_ERROR_CODE {
					cb := a.Value()
					if len(cb) >= 2 {
						errorCode = (int(cb[1]) << 8) + int(cb[0])
						log.Printf("AKA-Client-Error for Session ID: %s, code: %d", sid, errorCode)
					}
					break
				}
			}
			if err != nil {
				resultErr = fmt.Errorf(
					"AKA-Client-Error Packet for Session ID %s does not include AT_CLIENT_ERROR_CODE", sid)
				log.Printf("WARNING: %v", resultErr)
			}
		}
	} else {
		resultErr = fmt.Errorf("Missing CTX/Empty Session ID in AKA-Client-Error")
		log.Printf("WARNING: %v", resultErr)
	}
	return peerFailure(s, sid, req.Identifier(), errorCode), resultErr
}

// notificationResponse implements handler for EAP-Response/AKA-Notification
// see https://tools.ietf.org/html/rfc4187#section-9.11 for details
func notificationResponse(s *servicers.EapAkaSrv, ctx *protos.EapContext, req eap.Packet) (eap.Packet, error) {
	var (
		sid       string
		resultErr error
		errorCode int
	)

	if ctx == nil || len(ctx.SessionId) == 0 {
		log.Printf("WARNING: Missing CTX/Empty Session ID in AKA-Notification")
	} else {
		sid = ctx.SessionId
	}
	if len(req) < 12 { // min Notification packet len
		resultErr = fmt.Errorf("Session AKA-Notification for session ID %s is too short: %x", sid, req)
		log.Printf("WARNING: %v", resultErr)
	} else {
		scanner, err := eap.NewAttributeScanner(req)
		if err != nil {
			resultErr = fmt.Errorf("Malformed Session AKA-Notification for session ID %s: %x", sid, req)
			log.Printf("WARNING: %v", resultErr)
		} else {
			var a eap.Attribute
			for a, err = scanner.Next(); err == nil; a, err = scanner.Next() {
				if a.Type() == aka.AT_NOTIFICATION {
					cb := a.Value()
					if len(cb) >= 2 {
						if cb[0]&0x80 != 0 { // check S bit, it must be zero on error
							errorCode = int((uint16(cb[1]) << 8) + uint16(cb[0]))
							resultErr = fmt.Errorf("AKA-Notification S bit is set for Session ID: %s, code: %d",
								sid, errorCode)
							log.Printf("ERROR: %v", resultErr)
						}
					}
					break
				}
			}
			if err != nil {
				resultErr = fmt.Errorf("AKA-Notification Packet for Session ID %s does not include AT_NOTIFICATION",
					sid)
				log.Printf("WARNING: %v", resultErr)
			}
		}
	}
	return peerFailure(s, sid, req.Identifier(), errorCode), resultErr
}

func peerFailure(s *servicers.EapAkaSrv, sessionId string, identifier uint8, errorCode int) eap.Packet {
	if s != nil {
		imsi := s.RemoveSession(sessionId)
		if len(imsi) > 0 {
			log.Printf("EAP-AKA Peer failure for Session ID: %s, IMSI: %s, Error Code: %d",
				sessionId, imsi, errorCode)
		}
	}
	return []byte{eap.FailureCode, identifier + 1, 0, 4}
}
