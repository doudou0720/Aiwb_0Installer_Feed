#os=All
import sys, os
# sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))
import github

releases = [{
    'version-original': release['tag_name'],
    'version': release['tag_name'][1:],
    'released': release['published_at'][0:10]
} for release in github.releases('PyGithub/PyGithub') if "p" not in release['tag_name']]

# print("GET PyGithub/PyGithub",releases)