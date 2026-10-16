#!/bin/sh
# Tag the next version and push it, which starts the release workflow.
#
#   scripts/release.sh [patch|minor|major|vX.Y.Z] [-y]
set -eu

bump=patch
yes=0
for arg in "$@"; do
	case "$arg" in
		-y | --yes) yes=1 ;;
		patch | minor | major) bump=$arg ;;
		v[0-9]*.[0-9]*.[0-9]*) bump=$arg ;;
		*) echo "usage: $0 [patch|minor|major|vX.Y.Z] [-y]" >&2; exit 2 ;;
	esac
done

fail() { echo "error: $*" >&2; exit 1; }

branch=$(git rev-parse --abbrev-ref HEAD)
[ "$branch" = main ] || fail "releases are cut from main (you're on $branch)"
[ -z "$(git status --porcelain)" ] || fail "commit or stash your changes first"

git fetch --quiet --tags origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] ||
	fail "main differs from origin/main — push or pull first"

latest=$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n 1)
latest=${latest:-v0.0.0}

case "$bump" in
	v*) next=$bump ;;
	*)
		IFS=. read -r major minor patch <<EOF
${latest#v}
EOF
		patch=${patch%%-*}
		case "$bump" in
			major) next="v$((major + 1)).0.0" ;;
			minor) next="v$major.$((minor + 1)).0" ;;
			patch) next="v$major.$minor.$((patch + 1))" ;;
		esac
		;;
esac

git rev-parse -q --verify "refs/tags/$next" >/dev/null && fail "$next already exists"

if [ "$latest" = v0.0.0 ] || ! git rev-parse -q --verify "refs/tags/$latest" >/dev/null; then
	range=HEAD
else
	range="$latest..HEAD"
	[ -n "$(git log --oneline "$range")" ] || fail "nothing to release since $latest"
fi

echo "Releasing $next (previous: $latest)"
echo
git log --oneline --no-decorate "$range" | sed 's/^/  /'
echo

if [ "$yes" -ne 1 ]; then
	printf "Tag and push %s? [y/N] " "$next"
	read -r answer
	case "$answer" in
		y | Y | yes) ;;
		*) echo "Aborted"; exit 1 ;;
	esac
fi

git tag -a "$next" -m "$next"
git push --quiet origin "$next"

echo "Pushed $next — the release workflow is running:"
echo "  gh run watch --repo brimblehq/anarchy"
