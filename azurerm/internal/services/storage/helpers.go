package storage

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
)

var (
	storageAccountsCache = map[string]accountDetails{}

	accountsLock    = sync.RWMutex{}
	credentialsLock = sync.RWMutex{}
)

type accountDetails struct {
	ID            string
	ResourceGroup string
	Properties    *storage.AccountProperties

	accountKey *string
	name       string
}

func (ad *accountDetails) AccountKey(ctx context.Context, client Client) (*string, error) {
	credentialsLock.Lock()

	if ad.accountKey != nil {
		return ad.accountKey, nil
	}

	log.Printf("[DEBUG] Cache Miss - looking up the account key for storage account %q..", ad.name)
	props, err := client.AccountsClient.ListKeys(ctx, ad.ResourceGroup, ad.name)
	if err != nil {
		credentialsLock.Unlock()
		return nil, fmt.Errorf("Error Listing Keys for Storage Account %q (Resource Group %q): %+v", ad.name, ad.ResourceGroup, err)
	}

	if props.Keys == nil || len(*props.Keys) == 0 || (*props.Keys)[0].Value == nil {
		credentialsLock.Unlock()
		return nil, fmt.Errorf("Keys were nil for Storage Account %q (Resource Group %q): %+v", ad.name, ad.ResourceGroup, err)
	}

	keys := *props.Keys
	ad.accountKey = keys[0].Value

	credentialsLock.Unlock()

	return nil, nil
}

func (client Client) AddToCache(accountName string, props storage.Account) error {
	accountsLock.Lock()
	defer accountsLock.Unlock()

	account, err := client.populateAccountDetails(accountName, props)
	if err != nil {
		return err
	}

	storageAccountsCache[accountName] = *account

	return nil
}

func (client Client) RemoveAccountFromCache(accountName string) {
	accountsLock.Lock()
	delete(storageAccountsCache, accountName)
	accountsLock.Unlock()
}

func (client Client) FindAccount(ctx context.Context, accountName string) (*accountDetails, error) {
	accountsLock.Lock()
	defer accountsLock.Unlock()

	if existing, ok := storageAccountsCache[accountName]; ok {
		return &existing, nil
	}

	accounts, err := client.AccountsClient.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving storage accounts: %+v", err)
	}

	if accounts.Value == nil {
		return nil, fmt.Errorf("Error loading storage accounts: accounts was nil!")
	}

	for _, v := range *accounts.Value {
		if v.Name == nil {
			continue
		}

		account, err := client.populateAccountDetails(accountName, v)
		if err != nil {
			return nil, err
		}

		storageAccountsCache[*v.Name] = *account
	}

	if existing, ok := storageAccountsCache[accountName]; ok {
		return &existing, nil
	}

	return nil, nil
}

func (client Client) populateAccountDetails(accountName string, props storage.Account) (*accountDetails, error) {
	if props.ID == nil {
		return nil, fmt.Errorf("`id` was nil for Account %q", accountName)
	}

	accountId := *props.ID
	id, err := ParseAccountID(accountId)
	if err != nil {
		return nil, fmt.Errorf("Error parsing %q as a Resource ID: %+v", accountId, err)
	}

	return &accountDetails{
		name:          accountName,
		ID:            accountId,
		ResourceGroup: id.ResourceGroup,
		Properties:    props.AccountProperties,
	}, nil
}
