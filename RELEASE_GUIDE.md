# How to Release FastBrew

## Prerequisites
1.  **GitHub Account**: You need a GitHub account.
2.  **Repositories**:
    *   Create a repository named `fastbrew`.
    *   Create a repository named `homebrew-tap`.
3.  **GitHub Token**: Create a [Personal Access Token (Classic)](https://github.com/settings/tokens) with `repo` permissions.

## Setup Steps

1.  **Update Config**:
    Open `.goreleaser.yaml` and replace `YOUR_GITHUB_USERNAME` with your actual username in **3 places**.

2.  **Push Code**:
    ```bash
    git init
    git add .
    git commit -m "Initial commit"
    git branch -M main
    git remote add origin https://github.com/kasyap1234/fastbrew.git
    git push -u origin main
    ```

3.  **Install GoReleaser** (if not installed):
    ```bash
    brew install goreleaser/tap/goreleaser
    ```

## Releasing a New Version

1.  **Tag the release**:
    ```bash
    git tag -a v0.1.0 -m "First release"
    git push origin v0.1.0
    ```

2.  **Run GoReleaser**:
    ```bash
    export GITHUB_TOKEN="your_token_here"
    goreleaser release --clean
    ```

## How Users Will Install It

Once released, users can install it using your custom tap:

```bash
brew tap kasyap1234/tap
brew install fastbrew
```
