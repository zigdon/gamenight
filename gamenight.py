import os
import urllib

from google.appengine.api import users

import jinja2
import webapp2

from schema import GamenightNext, Gamenight, Application, User

JINJA_ENVIRONMNT = jinja2.Environment(
  loader=jinja2.FileSystemLoader(os.path.dirname(__file__)),
  extensions=['jinja2.ext.autoescape'])


class MainPage(webapp2.RequestHandler):

    def get(self):
        futurenights = Gamenight.future(10)
        gamenight = GamenightNext.query().fetch(1)[0]
        template_values = {
          'future': futurenights,
          'status': gamenight.status,
          'where': gamenight.location,
          'when': gamenight.time.strftime('%I:%M %p'),
          'updated': gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p'),
        }

        template = JINJA_ENVIRONMNT.get_template('index.html')
        # Write the submission form and the footer of the page
        self.response.write(template.render(template_values))


application = webapp2.WSGIApplication([
    ('/', MainPage),
], debug=True)

# vim: set ts=4 sts=4 sw=4 et:

