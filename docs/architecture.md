# Architecture

Waymark is the authority for the names in its book. A name has one or more IPv4 addresses and a deadline. `Lookup` drops addresses whose deadline has passed. When the last address is gone, the name is gone too.

A DNS query arrives as one UDP datagram of at most 512 bytes. The response copies the query id and the question. An A question for a live name gets one answer per address, with the remaining TTL in seconds. The answer name is a pointer back to the question. An A question for an unknown name is NXDOMAIN. Any other type for a known name is a successful reply with no answers.

`POST /v1/names/{name}` replaces the address list and the TTL. `DELETE` removes one address. Both the DNS socket and the HTTP socket stay on loopback.
