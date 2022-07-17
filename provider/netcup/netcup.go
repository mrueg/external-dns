/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package netcup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	nc "github.com/aellwein/netcup-dns-api/pkg/v1"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// NetcupProvider is an implementation of Provider for Netcup DNS.
type NetcupProvider struct {
	provider.BaseProvider
	client       *nc.NetcupDnsClient
	session      *nc.NetcupSession
	domainFilter endpoint.DomainFilter
	zoneIdFilter provider.ZoneIDFilter
	dryRun       bool
}

// NewNetcupProvider initializes a new Netcup provider.
func NewNetcupProvider(
	ctx context.Context,
	domainFilter *endpoint.DomainFilter,
	zoneIdFilter *provider.ZoneIDFilter,
	dryRun bool,
) (provider.Provider, error) {
	if !domainFilter.IsConfigured() || len(domainFilter.Filters) == 0 {
		return nil, fmt.Errorf("domain filter MUST be configured for use with Netcup provider")
	}
	customerNumber, exists := os.LookupEnv("NETCUP_CUSTOMER_NUMBER")

	if !exists || strings.TrimSpace(customerNumber) == "" {
		return nil, fmt.Errorf("this provider requires customer number to be set (via env variable \"NETCUP_CUSTOMER_NUMBER\")")
	} else {
		customerNo, err := strconv.Atoi(customerNumber)
		if err != nil {
			return nil, fmt.Errorf("customer number must be a numeric value (got '%v')", customerNumber)
		}
		apiKey, exists := os.LookupEnv("NETCUP_API_KEY")
		if !exists || strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("this provider requires API key to be set (via env variable \"NETCUP_API_KEY\")")
		}

		apiPassword, exists := os.LookupEnv("NETCUP_API_PASSWORD")
		if !exists || strings.TrimSpace(apiPassword) == "" {
			return nil, fmt.Errorf("this provider requires API password to be set (via env variable \"NETCUP_API_PASSWORD\")")
		}
		p := &NetcupProvider{
			domainFilter: *domainFilter,
			zoneIdFilter: *zoneIdFilter,
			client:       nc.NewNetcupDnsClient(customerNo, apiKey, apiPassword),
			dryRun:       dryRun,
		}
		return p, nil
	}
}

func (p *NetcupProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	if !changes.HasChanges() {
		log.Debugf("No changes detected - nothing to do")
		return nil
	}
	if !p.domainFilter.IsConfigured() || len(p.domainFilter.Filters) == 0 {
		return fmt.Errorf("domain filter MUST be configured for use with Netcup provider")
	}
	if err := p.ensureLogin(); err != nil {
		return err
	} else {
		defer p.session.Logout()
	}
	toCreate := p.mapChangesByZone(changes.Create)

	return nil
}

// Records delivers the list of Endpoint records for domains specified in domain filter.
// Note: for Netcup provider, the domain filter is mandatory, as it is not possible to obtain all of the
// zone records at once using Netcup API.
func (p *NetcupProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	if !p.domainFilter.IsConfigured() || len(p.domainFilter.Filters) == 0 {
		return nil, fmt.Errorf("domain filter MUST be configured for use with Netcup provider")
	}
	if err := p.ensureLogin(); err != nil {
		return nil, err
	} else {
		defer p.session.Logout()
	}
	endpoints := make([]*endpoint.Endpoint, 0)

	for _, domain := range p.domainFilter.Filters {
		// some information is on DNS zone itself, query it first
		zone, err := p.session.InfoDnsZone(domain)
		if err != nil {
			return nil, fmt.Errorf("unable to query DNS zone info for domain '%v': %v", domain, err)
		}
		ttl, err := strconv.ParseUint(zone.Ttl, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected error: unable to convert '%s' to uint64", zone.Ttl)
		}
		// query the records of the domain
		recs, err := p.session.InfoDnsRecords(domain)
		if err != nil {
			return nil, fmt.Errorf("unable to get DNS records for domain '%v': %v", domain, err)
		}
		log.Infof("got DNS records for domain '%v'", domain)
		for _, rec := range *recs {
			ep := endpoint.NewEndpointWithTTL(fmt.Sprintf("%s.%s", rec.Hostname, domain), rec.Type, endpoint.TTL(ttl), rec.Destination).
				WithSetIdentifier(rec.Id).
				WithProviderSpecific("priority", rec.Priority)
			log.Debugf("found record: %v", ep)
			endpoints = append(endpoints, ep)
		}
	}
	return endpoints, nil
}

func (p *NetcupProvider) mapChangesByZone(changes []*endpoint.Endpoint) map[string][]*endpoint.Endpoint {
	result := make(map[string][]*endpoint.Endpoint)
	for _, change := range changes {

	}
	return result
}

// ensureLogin makes sure that we are logged in to Netcup API.
func (p *NetcupProvider) ensureLogin() error {
	var err error
	log.Debug("performing login to Netcup DNS API")
	if p.session, err = p.client.Login(); err != nil {
		return err
	} else {
		log.Debug("successfully logged in to Netcup DNS API")
	}
	return err

}
