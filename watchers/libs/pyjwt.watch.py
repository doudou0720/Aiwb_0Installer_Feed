#os=All
import sys, os
import github

releases = [{
    'version-original': release['tag_name'],
    'version': release['tag_name'][1:] if release['tag_name'].startswith('v') else release['tag_name'],
    'released': release['published_at'][0:10]
} for release in github.releases('jpadilla/pyjwt') if "b" not in release['tag_name'].lower() and "a" not in release['tag_name'].lower()]

