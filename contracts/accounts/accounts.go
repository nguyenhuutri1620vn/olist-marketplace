// Package accounts holds the identifiers of seeded demo Accounts (FR15), shared
// by identity-service and the importer.
package accounts

// DemoDomain is reserved for seeded Accounts; registration rejects it.
const DemoDomain = "demo.olist.local"

// AdminAccountID is the fixed id of the single seeded Admin, so its import is
// an idempotent upsert like the dataset Accounts.
const AdminAccountID = "0192f000-0000-7000-8000-00000000ad01"

// AdminEmail is the seeded Admin's login.
const AdminEmail = "admin@" + DemoDomain
