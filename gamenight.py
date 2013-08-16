from datetime import datetime
import jinja2
import logging
import os
import urllib
import webapp2

from google.appengine.api import users

from schema import Gamenight, Application, User
from utils import Utils


JINJA_ENVIRONMNT = jinja2.Environment(
  loader=jinja2.FileSystemLoader(os.path.dirname(__file__)),
  extensions=['jinja2.ext.autoescape'])

class MainPage(webapp2.RequestHandler):
    def get(self):
        futurenights = Gamenight.future(10)

        if futurenights and futurenights[0].this_week():
            # we have a gamenight scheduled for this week
            gamenight = futurenights[0]
            if len(futurenights) > 1:
                futurenights = futurenights[1:]
            else:
                futurenights = None
        else:
            gamenight = Gamenight.schedule()

        updated = gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p')
        template_values = {
          'future': futurenights,
          'status': gamenight.status,
          'updated': updated,
        }

        if gamenight.date:
            template_values['when'] = gamenight.date.strftime('%I:%M %p')

        if gamenight.location:
            template_values['where'] =  gamenight.location

        template = JINJA_ENVIRONMNT.get_template('index.html')
        # Write the submission form and the footer of the page
        self.response.write(template.render(template_values))


class EditPage(webapp2.RequestHandler):
    def get(self):
        sys_user = users.get_current_user()
        if not sys_user:
            self.redirect(users.create_login_url(self.request.uri))
            return

        user = User.get_or_insert(sys_user.email())

        futurenights = Gamenight.future(100)
        template_values = {
            'scheduled': futurenights,
            'logout': users.create_logout_url('/'),
            'user': user.key.id(),
            'admin': user.superuser,
            'users': User.query().fetch(100),
        }

        template = JINJA_ENVIRONMNT.get_template('edit.html')
        self.response.write(template.render(template_values))


application = webapp2.WSGIApplication([
    ('/', MainPage),
    ('/edit', EditPage),
], debug=True)

# vim: set ts=4 sts=4 sw=4 et:
