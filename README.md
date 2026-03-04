# Panther CLI

The `panther-cli` repository contains CLI tooling written in [Go][0] for administering and provisioning
Panther instances.

At the time of writing (February 2025), the following tooling is available:

- `panther-cloud-connected-setup` - a tool for setting up new Snowflake and AWS accounts for a
  [Panther Cloud Connected][2] deployment.

## Building the Tools

This repository uses [`justfile`][6] in the same way one might use a Makefile. Useful commands
can be found in the file, but most likely all you'll need is: `just bf`.

Releases for the tool will be automated using [goreleaser][7].

## Overview: `panther-cloud-connected-setup`

The `panther-cloud-connected-setup` tool automates the initial provisioning steps for Snowflake
and AWS for new Cloud Connected Deployments. Specifically, it automates many of the steps outlined
[here][2], as follows:

- (**optional**) create a new Snowflake account within a Snowflake organization
- deploy the AWS IAM role `PantherDeploymentRole` to the target AWS account
- deploy the Pre-Deployment Tooling to the target AWS account
- execute the `PantherReadinessCheck` Pre-Deployment Tool
- validate Snowflake account credentials for connectivity
- register for SSL certificates for the following subdomains based on your provided root domain:
  - `<desired panther subdomain>.yourdomain.com`
  - `*.<desired panther subdomain>.yourdomain.com`

These steps are straightforward but complex and error prone. This tool aims to reduce the
friction of setting up a new Cloud Connected account, wherever possible.

### Using `panther-cloud-connected-setup`

To use the tool, you'll need to create a config file. We've provided an example config file
here: [example-config-existing-snowflake-acct.yml](example-config-existing-snowflake-acct.yml) or
[example-config-new-snowflake-acct.yml](example-config-new-snowflake-acct.yml).


#### Running the Tool with a Config

```bash
./panther-cloud-connected-setup --config-file config.yml
```

#### With Verbose Logging

```bash
./panther-cloud-connected-setup --config-file config.yml --verbose
```

#### With Verbose *Snowflake* Logging

```bash
./panther-cloud-connected-setup --config-file config.yml --snowflake-logging
```

#### Show Last Run's Results

```bash
./panther-cloud-connected-setup --show-last-run
```


#### Run Only the AWS Readiness Check

Use `--only-readiness-check` to invoke the `PantherReadinessCheck` Lambda and exit immediately,
skipping datalake setup and all certificate steps. This is useful for validating AWS IAM
permissions and diagnosing issues caused by Service Control Policies (SCPs) or Control Tower
policies without running the full provisioning flow. The check is always re-run, even if a
previous run already recorded a passing result.

```bash
./panther-cloud-connected-setup --config-file config.yml --only-readiness-check
```

#### Cleaning Local State

```bash
./panther-cloud-connected-setup --clean
```


### Important Notes about `panther-cloud-connected-setup`

- The tool performs all of the above operations from your local machine OR within *your*
  AWS or Snowflake account. No credentials or information are sent to Panther during the use
  of this tool.
- State is stored for this tool in a file called `panther-cli-state.db`. This state
  database simplifies re-runs by tracking which steps have successfully completed.
  - **Sensitive information is stored in this file. It is recommended you run
    `panther-cloud-connected-setup --clean` upon successful provision to purge this file.** You may
    also simply delete the file from disk. No other state is written to disk.
- ***You will still need to complete steps [8-10 on this page][3].***

The following subsection provides specific notes about the expectations the tooling has.

### Snowflake Notes

- If you're allowing the tool to create your Snowflake account for you, the user specified for `ORGADMIN` credentials must
  have the fields `NAME` and `LOGIN_NAME` match. To check this, you can use the following command in the Snowflake Console
  in a SQL worksheet:

    ```sql
    DESC USER <your user>; -- update the username here
    SELECT "property", "value"
        FROM TABLE(RESULT_SCAN(LAST_QUERY_ID()))
        WHERE "property" = 'NAME' OR "property" = 'LOGIN_NAME';
    ```
- This tool only supports RSA KeyPair authentication. Please see the [Snowflake Documentation][1]
  for more information about setting an RSA KeyPair for your `ORGADMIN` user.

### AWS Notes

- You must create a new AWS account yourself.
- You will need to register a root domain or re-use an existing domain to host your Panther Instance off of.
- The IAM permissions you provide in your configuration file should ideally have root-level access, but at
  a minimum, must have the following permissions:
  - ability to deploy CloudFormation templates
  - create ACM Certificates
  - create and invoke Lambdas
  - read/write to SecretsManager
- You may add the AWS account to an organization, but [Service Control Policies (SCP)][4]
  and [Control Tower][5] policies can interfere with a successful provision. ***It is best to
  provision first, then build your SCP or Control Tower policies around the `PantherDeploymentRole`
  once deployed.***

[0]: https://go.dev
[1]: https://docs.snowflake.com/en/user-guide/key-pair-auth#configuring-key-pair-authentication
[2]: https://docs.panther.com/system-configuration/panther-deployment-types/cloud-connected
[3]: https://docs.panther.com/system-configuration/panther-deployment-types/cloud-connected/aws#step-8-provide-values-to-panther
[4]: https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_scps.html
[5]: https://docs.aws.amazon.com/audit-manager/latest/userguide/controltower.html
[6]: https://just.systems/
[7]: https://goreleaser.com/

### Development

For Panther devs:

After merging your changes to main in this repository, the release workflow automatically opens a PR in the [homebrew taps repository](https://github.com/panther-labs/homebrew-taps). Approve and merge that PR so people installing via Homebrew get the latest version.
