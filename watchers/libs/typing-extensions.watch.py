#os=All
import sys, os
import github

releases = [{
    'version-original': tag['name'],
    'version': tag['name'][1:] if tag['name'].startswith('v') else tag['name'],
    'released': ''
} for tag in github.tags('python/typing-extensions')]

