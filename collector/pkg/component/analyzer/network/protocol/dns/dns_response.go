package dns

import (
	"net"
	"strings"

	"github.com/Kindling-project/kindling/collector/pkg/component/analyzer/network/protocol"
	"github.com/Kindling-project/kindling/collector/pkg/model/constlabels"
)

const (
	TypeA    uint16 = 1
	TypeAAAA uint16 = 28
)

func fastfailDnsResponse() protocol.FastFailFn {
	return func(message *protocol.PayloadMessage) bool {
		return len(message.Data) <= DNSHeaderSize || len(message.Data) > MaxMessageSize
	}
}

/**
Header
   0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
 +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
 |                      ID                       |
 +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
 |QR|   Opcode  |AA|TC|RD|RA|   Z    |   RCODE   |
 +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
 |                    QDCOUNT                    |
 +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
 |                    ANCOUNT                    |
 +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
 |                    NSCOUNT                    |
 +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
 |                    ARCOUNT                    |
 +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
*/
func parseDnsResponse() protocol.ParsePkgFn {
	return func(message *protocol.PayloadMessage) (bool, bool) {
		offset := message.Offset
		_, id := message.ReadUInt16(offset)
		_, flags := message.ReadUInt16(offset + 2)

		qr := (flags >> 15) & 0x1
		opcode := (flags >> 11) & 0xf
		rcode := flags & 0xf

		_, numOfQuestions := message.ReadUInt16(offset + 4)
		_, numOfAnswers := message.ReadUInt16(offset + 6)
		_, numOfAuth := message.ReadUInt16(offset + 8)
		_, numOfAddl := message.ReadUInt16(offset + 10)
		numOfRR := numOfQuestions + numOfAnswers + numOfAuth + numOfAddl

		/*
			Kind of query in this message
				0	a standard query (QUERY)
				1	an inverse query (IQUERY)
				2	a server status request (STATUS)
				3-15 	reserved for future use

			Response code
				0	No error condition
				1 	Format error
				2 	Server failure
				3	Name Error
				4 	Not Implemented
				5 	Refused
				6-15 	Reserved for future use.
		*/
		if qr == 0 || opcode > 2 || rcode > 5 || numOfQuestions == 0 || numOfRR > MaxNumRR {
			return false, true
		}

		domain, err := readQuery(message, numOfQuestions)
		if err != nil {
			return false, true
		}

		ip := readIpV4Answer(message, numOfAnswers)

		message.AddStringAttribute(constlabels.DnsDomain, domain)
		if len(ip) > 0 {
			message.AddStringAttribute(constlabels.DnsIp, ip)
		}
		message.AddIntAttribute(constlabels.DnsId, int64(id))
		message.AddIntAttribute(constlabels.DnsRcode, int64(rcode))
		if rcode > 0 {
			message.AddBoolAttribute(constlabels.IsError, true)
			message.AddIntAttribute(constlabels.ErrorType, int64(constlabels.ProtocolError))
		}
		return true, true
	}
}

func readIpV4Answer(message *protocol.PayloadMessage, answerCount uint16) string {
	var (
		complete bool
		aType    uint16
		length   uint16
		ip       net.IP
		ips      []string
	)

	ips = make([]string, 0)
	offset := message.Offset
	for i := 0; i < int(answerCount); i++ {
		/*
			uint16 name
			uint16 type
			uint16 class
			uint32 ttl
			uint16 rdlength
			string rdata
		*/
		offset += 2
		complete, aType = message.ReadUInt16(offset)
		if complete {
			break
		}

		offset += 8
		complete, length = message.ReadUInt16(offset)
		if complete {
			break
		}

		offset += 2
		if aType == TypeA {
			offset, ip = message.ReadBytes(offset, int(length))
			if ip == nil {
				break
			}
			ips = append(ips, ip.String())
		}
		offset += int(length)
	}
	message.Offset = offset
	if len(ips) == 0 {
		return ""
	}

	return strings.Join(ips, ",")
}
