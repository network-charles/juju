Thanks for your interest in Juju -- contributions like yours make good projects
great!

Whether it is code or docs, there are two basic ways to contribute: by opening
an issue or by creating a PR. This document gives detailed information about
both.

> Note: If at any point you get stuck, come chat with us on
[Matrix](https://matrix.to/#/#charmhub-juju:ubuntu.com).

## Open an issue

You will need a GitHub account ([sign up](https://github.com/signup)).

### Open an issue for docs

To open an issue for a specific doc, find it in [the published
docs](https://documentation.ubuntu.com/juju),
then use the **Give feedback** button.

To open an issue for docs in general, do the same for the homepage of the docs
 or go to https://github.com/juju/juju/issues , click on **New issue** (top
right corner of the page), select “Documentation issue”, then fill out the issue
template and submit the issue.

### Open an issue for code

Go to https://github.com/juju/juju/issues  click on **New issue** (top right
corner of the page), select whatever is appropriate, then fill out the issue
template and submit the issue.

> Note: For feature requests please use
https://matrix.to/#/#charmhub-juju:ubuntu.com

## Make your first contribution

You will need a GitHub account ([sign up](https://github.com/signup) and [add
your public SSH key](https://github.com/settings/ssh)) and `git` ([get
started](https://git-scm.com/book/en/v2/Getting-Started-What-is-Git%3F)).

Then:

1. [Sign the Canonical Contributor Licence Agreement
   (CLA)](https://ubuntu.com/legal/contributors).

2. Configure your `git` so your commits are signed:

```
git config --global user.name "A. Hacker"
git config --global user.email "a.hacker@example.com"
git config --global commit.gpgsign true
```

> See more: [GitHub | Authentication > Signing commits](https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits)

3. Fork juju/juju. This will create `https://github.com/<user>/juju`.

4. Clone your fork locally and enter the repo.

```
git clone git@github.com:<user>/juju.git
cd juju
```

5. Add a new remote with the name `upstream` and set it to point to the upstream
   `juju` repo.

```
git remote add upstream git@github.com:juju/juju.git
```

6. Set your local branches to track the `upstream` remote (not your fork). E.g.,

```
git fetch --all
git checkout 3.6
git branch --set-upstream-to=upstream/3.6
git checkout main
git branch --set-upstream-to=upstream/main
```

7. Sync your local branches with the upstream, then check out the branch you
   want to contribute to and create a feature branch based on it. **If your
   contribution is not specific to a particular branch, please target the lowest that
   applies.** (All patches in earlier versions are eventually merged through to later
   versions.) E.g., for a change that should go into both Juju 3.6 and Juju 4
   (`main`):

```
git fetch upstream
git checkout 3.6
git pull
git checkout -b 3.6-new-stuff # your feature branch
```

8. Make the desired changes. Test changes locally.


----------------
<details>

<summary>Further info: Docs</summary>

The documentation is in `juju/docs`.

If you create a new page make sure to index it appropriately in the correct
overview page (usually, an index page in the directory where you've created the
page). If you delete a page, make sure to set up a redirect in the
`juju/docs/redirects.txt` file.

### Standards

All changes should follow the existing patterns, including
[Diátaxis](https://diataxis.fr), the [Canonical Documentation Style
Guide](https://docs.ubuntu.com/styleguide/en), the modular structure, the
cross-referencing pattern, [MyST
Markdown](https://canonical-documentation-with-sphinx-and-readthedocscom.readthedocs-hosted.com/style-guide-myst/),
etc.

### Testing

Changes should be inspected by building the docs and fixing any issues
discovered that way. To preview the docs as they will be rendered on RTD, in
`juju/docs` run `make run` and open the provided link in a browser. If you get
errors, try `make clean`, then `make run` again. For other checks, see `make
[Tab]` and select the command for the desired check.

</details>

----------------

----------------
<details>

<summary>Further info: Code</summary>

### Installing Go

`juju` is written in [Go](https://go.dev/). To install Go see [Go
docs](https://golang.org/doc/install#install).

### Building Juju and its dependencies

Fork and clone the Juju repo, then navigate to the root directory and run `make
install`:

```
git clone https://github.com/<user>/juju.git
cd juju
make install
```

### Updating Go dependencies

Juju uses Go modules to manage dependencies. To update a dependency, use the
following, ensuring that the dependency is using a version where possible, or a
commit hash if not available:


```
go get -u github.com/the/dependency@v1.2.3
go mod tidy
```

### Code formatting

To format your code, run `go fmt`.

> Note: Your editor may do this automatically.


### Imports

Import statements are grouped into 3 sections: standard library, 3rd party
libraries, juju imports. The tool "go fmt" can be used to ensure each
group is alphabetically sorted. eg:

```go
    import (
        "fmt"
        "time"

        "labix.org/v2/mgo"
        "github.com/juju/loggo"
        gc "gopkg.in/check.v1"

        "github.com/juju/juju/state"
        "github.com/juju/worker/v3"
    )
```

### Testing

Some tests may require local lxd to be installed, see
[installing lxd via snap](https://stgraber.org/2016/10/17/lxd-snap-available/).

Juju uses the `gocheck` testing framework, which is automatically installed
as a dependency of `juju`. You can read more about `gocheck` at
http://godoc.org/gopkg.in/check.v1. `gocheck` is integrated into the source of
each package so the standard `go test` command is used to run `gocheck` tests.
For example:

```
go test -v github.com/juju/juju/core/config -check.v
```

By default `gocheck` will run all tests
in a package, selected tests can by run by passing `-gocheck.f` to match a
subset of test names.

```
go test -gocheck.f '$REGEX'
```


### Testing and MongoDB

Many tests use a standalone instance of `mongod` as part of their setup. The
`mongod` binary found in `$PATH` is executed by these suites.  If you don't
already have MongoDB installed, run

```
make install-mongo-dependencies
```

### Other

For more information see [CODING.md](CODING.md)

</details>

----------------



9. As you make your changes, ensure that you always remain in sync with the upstream:

```
git pull upstream 3.6 --rebase
```

10. Stage, commit and push regularly to your fork. Make sure your commit messages
    comply with conventional commits ([see upstream
    standard](https://www.conventionalcommits.org/en/v1.0.0/), [see adaptation in
    Juju](./docs/contributor/reference/conventional-commits.md)). E.g.,

```
git add .
git commit -m "docs: add setup and teardown anchors"
git push origin 3.6-new-stuff
```

> Note: For most code PRs, it's best to type just `git commit`, then return; the
terminal will open a text editor, enabling you to write a lengthier, more
explicit message.

> Tip: If you've set things up correctly, typing just `git push` and returning
may be enough for `git` to prompt you with the correct arguments.

> Tip: If you don't want to create a new commit message every time, do
`git commit --amend --no-edit`, then `git push --force`.

11. Create the PR. In the PR window make sure to select the correct target base
    branch. In your PR description make sure to comply with the template rules (e.g.,
    explain _why_ you're making the change). If your change should target multiple
    branches, add a note at the top of your PR to say so (e.g., "This PR should be
    merged into both `3.6` and `main`").

12. Ensure GitHub tests pass.

13. In [the Matrix Juju Development
    channel](https://matrix.to/#/#charmhub-jujudev:ubuntu.com), drop a link to your
    PR with the mention that it needs reviews. Someone will review your PR. Make all
    the requested changes.

14. When you've received two approvals, your PR can be merged. If you are part
    of the `juju` organization, at this point in the Conversation view of your PR
    you can type `/merge` to merge. If not, ping one of your reviewers and ask them
    to help merge.

> Tip: After your first contribution, you will only have to repeat steps 7-14.

Congratulations and thank you!
