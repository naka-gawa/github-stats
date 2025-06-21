# github-stats

Visualize all your hard work on GitHub!

`github-stats` is a powerful Go-based CLI tool that aggregates your contributions (commits, created PRs, and reviewed PRs) for each repository within a specified GitHub Organization and outputs the result in JSON format.

## Features

Per-Repository Activity Breakdown: See exactly how much you've contributed to each repository at a glance!

Visualize Contributions with 3 Key Metrics:

- Commits
- Pull Requests Created
- Pull Requests Reviewed

Flexible Date Ranges: Use the --from and --to flags to narrow down the aggregation period.

JSON Output: Results are provided in JSON, making it easy to pipe to jq or other tools for further analysis.

Verbose Mode: Add the -v flag to see detailed logs of what's happening behind the scenes.

Rate Limit Aware: Intelligently adjusts request rates to avoid hitting the GitHub API rate limits.

## Installation

If you have a Go environment set up, you can install it with a single command:

```shell
go install [github.com/naka-gawa/github-stats@latest](https://github.com/naka-gawa/github-stats@latest)
```

## Usage

It's very simple to use!

## Set your GitHub Personal Access Token as an environment variable

```shell
export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"
```

## Basic usage

```shell
github-stats stats --org [ORGANIZATION_NAME] --user [YOUR_GITHUB_ID]
```

## Aggregate stats for a specific period

```shell
github-stats stats --org naka-gawa --user naka-gawa --from 2025/04/01 --to 2025/06/30
```

## Run with verbose logging

```shell
github-stats stats --org naka-gawa --user naka-gawa -v
```

## Authentication

This tool requires a Personal Access Token (PAT) to communicate with the GitHub API.

1. Generate a new Fine-grained personal access token from this page.
1. Set the Resource owner to the organization you want to aggregate stats for.
1. Under Repository permissions, grant Read-only access to the following:

- Contents
- Pull requests

1. Copy the generated token (ghp_...) and set it as an environment variable named GITHUB_TOKEN.

```shell
export GITHUB_TOKEN="YOUR_NEW_TOKEN_HERE"
```

## Example Output

The command prints a clean JSON array to standard output.

```shell
[
  {
    "name": "naka-gawa/xxxxx",
    "commits": 15,
    "created_prs": 3,
    "reviewed_prs": 8
  },
  ~snip~
]
```

## Contributing

Bug reports, feature requests, and pull requests are all welcome!
Feel free to open an issue to start a discussion.

To run tests:

```shell
go test -v ./...
```

## License

This project is released under the MIT License.
